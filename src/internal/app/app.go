package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/address"
	"github.com/gracesolutions/dns-automatic-updater/internal/cleanup"
	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/azure"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/cloudflare"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/powerdns"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/route53"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/technitium"
	"github.com/gracesolutions/dns-automatic-updater/internal/reconcile"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
	"github.com/gracesolutions/dns-automatic-updater/internal/scheduler"
	"github.com/gracesolutions/dns-automatic-updater/internal/service"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
	"github.com/gracesolutions/dns-automatic-updater/internal/watcher"
)

const version = "dev"

type Application struct {
	logger         *logging.Logger
	serviceManager service.Manager
	stdout         io.Writer
	stderr         io.Writer
}

// runState holds the mutable configuration and provider state that
// may be swapped when the config file is modified at runtime.
type runState struct {
	mu        sync.RWMutex
	cfg       config.Config
	providers map[string]core.Provider
}

func Main(args []string, stdout, stderr io.Writer) int {
	logger := logging.New(stderr, logging.LevelInformation)
	app := Application{
		logger:         logger,
		serviceManager: service.NewPlatformManager(logger),
		stdout:         stdout,
		stderr:         stderr,
	}
	if err := app.Run(args); err != nil {
		logger.Error(err.Error())
		return 1
	}
	return 0
}

func (a Application) Run(args []string) error {
	command, err := Parse(args)
	if err != nil {
		return err
	}
	switch command.Kind {
	case CommandRun:
		return a.run(command)
	case CommandService:
		return a.handleService(command)
	case CommandVersion:
		a.logger.Information(fmt.Sprintf("dnsreconciler version %s (%s/%s)", version, runtime.GOOS, runtime.GOARCH))
		return nil
	default:
		return fmt.Errorf("unsupported command %q", command.Kind)
	}
}

func (a Application) run(command Command) error {
	// Auto-create config if it doesn't exist
	if _, err := os.Stat(command.ConfigPath); os.IsNotExist(err) {
		a.logger.Information(fmt.Sprintf("Config file %q not found — generating default config", command.ConfigPath))
	}

	// §21.1 steps 1-2: load and validate config
	cfg, err := config.Load(command.ConfigPath)
	if err != nil {
		return err
	}

	// Apply CLI/env interval override before anything reads it
	if command.OverrideInterval > 0 {
		a.logger.Information(fmt.Sprintf("Overriding reconcile interval: %ds → %ds",
			cfg.Settings.Runtime.ReconcileIntervalSeconds, command.OverrideInterval))
		cfg.Settings.Runtime.ReconcileIntervalSeconds = command.OverrideInterval
	}

	// §21.1 step 3: initialize centralized logger
	a.logger.SetLevel(logging.ParseLevel(cfg.Settings.Runtime.LogLevel))

	// §21.1 step 4: load local state
	storePath := cfg.Settings.Runtime.StatePath
	if command.OverrideState != "" {
		storePath = command.OverrideState
	}
	store := state.JSONStore{Path: storePath}

	// Mutable state that may be reloaded when the config file changes.
	rs := &runState{
		cfg:       cfg,
		providers: a.buildProviderMap(cfg),
	}

	// reconcileOnce performs a single reconciliation pass (§21.1 steps 5-10).
	reconcileOnce := func(ctx context.Context) error {
		rs.mu.RLock()
		currentCfg := rs.cfg
		currentProviders := rs.providers
		rs.mu.RUnlock()

		rt := &currentCfg.Settings.Runtime
		net := &currentCfg.Settings.Network

		// Refresh provider capabilities if stale (e.g. Cloudflare plan detection).
		for _, p := range currentProviders {
			if refresher, ok := p.(core.CapabilityRefresher); ok {
				if err := refresher.RefreshCapabilitiesIfStale(ctx); err != nil {
					a.logger.Warning(fmt.Sprintf("Failed to refresh capabilities for %q: %s", p.Name(), err))
				}
			}
		}

		st, err := store.Load(ctx)
		if err != nil {
			return err
		}

		// §21.1 step 5: resolve runtime context
		rtResolver := runtimectx.NewDefaultResolver(a.logger, command.NodeID)
		snap, err := rtResolver.Resolve(ctx)
		if err != nil {
			return fmt.Errorf("runtime context resolution failed: %w", err)
		}

		// §21.1 step 7: merge defaults
		mergedRecords := config.MergeAllDefaults(&currentCfg)

		// §21.1 steps 6, 8-9: resolve addresses, expand, reconcile
		addrResolver := address.NewDefaultResolver(a.logger)

		reconciler := reconcile.Reconciler{
			Logger:          a.logger,
			Providers:       currentProviders,
			AddressResolver: addrResolver,
			Snapshot:        snap,
			GlobalSources:   net.AddressSources,
			DryRun:          rt.DryRun,
		}

		stats, _ := reconciler.ReconcileAll(ctx, mergedRecords, &st)

		// Prune orphaned state entries for records no longer in config
		activeIDs := make(map[string]struct{}, len(mergedRecords))
		for _, r := range mergedRecords {
			activeIDs[r.RecordID] = struct{}{}
		}
		if pruned := st.PruneOrphans(activeIDs); pruned > 0 {
			a.logger.Information(fmt.Sprintf("Pruned %d orphaned state entries", pruned))
		}

		// §21.1 step 10: persist state
		st.NodeID = snap.NodeID
		st.Hostname = snap.Hostname
		st.PublicIPv4Last = snap.PublicIPv4
		st.PublicIPv6Last = snap.PublicIPv6
		if err := store.Save(ctx, st); err != nil {
			return fmt.Errorf("state save failed: %w", err)
		}

		if stats.Errors > 0 {
			a.logger.Warning(fmt.Sprintf("Reconciliation completed with %d error(s)", stats.Errors))
		}
		return nil
	}

	// §25: single-pass mode vs continuous scheduler
	if command.Once {
		a.logger.Information("Running single reconciliation pass (--once)")
		return reconcileOnce(context.Background())
	}

	// Continuous mode: use scheduler with signal-driven shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start config file watcher
	fw := watcher.New(command.ConfigPath, a.logger)
	if err := fw.Init(); err != nil {
		a.logger.Warning(fmt.Sprintf("Config watcher init failed (continuing without): %s", err))
	}
	configChanged := fw.Watch(ctx)

	// Launch goroutine that reloads config when the file changes.
	go a.watchConfigReload(ctx, command, rs, configChanged)

	sched := scheduler.New(a.logger, scheduler.Config{
		IntervalSeconds: cfg.Settings.Runtime.ReconcileIntervalSeconds,
		JitterPercent:   10,
	}, reconcileOnce)

	a.logger.Information(fmt.Sprintf("Starting scheduler (interval=%ds)", cfg.Settings.Runtime.ReconcileIntervalSeconds))
	err = sched.Run(ctx)
	if err != nil && err != context.Canceled {
		return err
	}
	a.logger.Information("Scheduler stopped gracefully")

	// §26.1: cleanup on graceful shutdown
	rs.mu.RLock()
	cleanupOnShutdown := rs.cfg.Settings.Runtime.CleanupOnShutdown
	cleanupProviders := rs.providers
	rs.mu.RUnlock()

	if cleanupOnShutdown {
		a.logger.Information("CleanupOnShutdown is enabled — deleting owned records")
		cleaner := cleanup.Cleaner{
			Logger:    a.logger,
			Providers: cleanupProviders,
			Store:     &store,
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := cleaner.Run(cleanupCtx); err != nil {
			a.logger.Error(fmt.Sprintf("Cleanup failed: %s", err))
		}
	}

	return nil
}

// watchConfigReload listens for config file changes and reloads configuration
// and providers. Invalid configs are logged and skipped — the previous valid
// config continues to be used.
func (a Application) watchConfigReload(ctx context.Context, command Command, rs *runState, changes <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-changes:
			if !ok {
				return
			}
			a.logger.Information("Reloading configuration...")

			newCfg, err := config.Load(command.ConfigPath)
			if err != nil {
				a.logger.Error(fmt.Sprintf("Config reload failed (keeping current config): %s", err))
				continue
			}

			// Re-apply CLI overrides
			if command.OverrideInterval > 0 {
				newCfg.Settings.Runtime.ReconcileIntervalSeconds = command.OverrideInterval
			}

			a.logger.SetLevel(logging.ParseLevel(newCfg.Settings.Runtime.LogLevel))
			newProviders := a.buildProviderMap(newCfg)

			rs.mu.Lock()
			rs.cfg = newCfg
			rs.providers = newProviders
			rs.mu.Unlock()

			a.logger.Information(fmt.Sprintf("Configuration reloaded successfully (%d providers, %d records)",
				len(newCfg.Providers), len(newCfg.Records)))
		}
	}
}

// buildProviderMap creates provider instances from the providers array,
// keyed by provider ID for direct lookup from record templates (§9).
func (a Application) buildProviderMap(cfg config.Config) map[string]core.Provider {
	registry := provider.NewRegistry()
	registry.Register("cloudflare", cloudflare.New)
	registry.Register("technitium", technitium.New)
	registry.Register("powerdns", powerdns.New)
	registry.Register("route53", route53.New)
	registry.Register("azure", azure.New)

	providers := map[string]core.Provider{}
	for _, entry := range cfg.Providers {
		if !entry.IsEnabled() {
			a.logger.Information(fmt.Sprintf("Provider %q is disabled, skipping", entry.ID))
			continue
		}
		provCfg := entry.RawConfig
		if provCfg == nil {
			provCfg = map[string]any{}
		}
		p, err := registry.Build(entry.Type, provCfg, a.logger)
		if err != nil {
			a.logger.Error(fmt.Sprintf("Failed to initialize provider %q (%s): %s", entry.ID, entry.Type, err))
			continue
		}
		providers[entry.ID] = p
		label := entry.FriendlyName
		if label == "" {
			label = entry.ID
		}
		a.logger.Information(fmt.Sprintf("Provider %s [ProviderID: %s] [Provider: %s] initialized successfully", label, entry.ID, entry.Type))
	}
	return providers
}

func (a Application) handleService(command Command) error {
	ctx := context.Background()
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	options := service.Options{
		Name:        command.ServiceName,
		DisplayName: "DNS Reconciler",
		Description: "Dynamic DNS reconciliation service",
		BinaryPath:  exePath,
	}
	switch command.ServiceAction {
	case service.ActionInstall:
		return a.serviceManager.Install(ctx, options)
	case service.ActionUninstall:
		return a.serviceManager.Uninstall(ctx, options)
	case service.ActionStart:
		return a.serviceManager.Start(ctx, options)
	case service.ActionStop:
		return a.serviceManager.Stop(ctx, options)
	case service.ActionInit:
		if err := a.serviceManager.Install(ctx, options); err != nil {
			return err
		}
		return a.serviceManager.Start(ctx, options)
	default:
		return fmt.Errorf("unsupported service action %q", command.ServiceAction)
	}
}