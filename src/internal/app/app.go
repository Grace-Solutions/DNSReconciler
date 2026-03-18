package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

// Version is set at build time via -ldflags "-X ...app.Version=yyyy.mm.dd.hhmm".
// Falls back to "dev" for untagged / local builds.
var Version = "dev"

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
		a.logger.Information(fmt.Sprintf("dnsreconciler version %s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH))
		return nil
	default:
		return fmt.Errorf("unsupported command %q", command.Kind)
	}
}

// resolveConfigTTL determines the remote config TTL from CLI flag, config file,
// or the default (1h). CLI flag takes priority over config file.
func resolveConfigTTL(cliTTL string, cfgRemote *config.RemoteConfig) time.Duration {
	const defaultTTL = 1 * time.Hour

	// CLI flag takes priority
	if cliTTL != "" {
		if d, err := time.ParseDuration(cliTTL); err == nil && d > 0 {
			return d
		}
	}
	// Config file setting
	if cfgRemote != nil && cfgRemote.TTL != "" {
		if d, err := time.ParseDuration(cfgRemote.TTL); err == nil && d > 0 {
			return d
		}
	}
	return defaultTTL
}

func (a Application) run(command Command) error {
	var cfg config.Config
	var err error
	var remoteCache *config.RemoteCache

	remoteReq := config.RemoteRequest{
		URL:    command.ConfigURL,
		Method: command.ConfigMethod,
		Header: command.ConfigHeader,
		Token:  command.ConfigToken,
	}

	if command.ConfigURL != "" {
		// Fetch configuration from remote URL
		a.logger.Information(fmt.Sprintf("Fetching configuration from %s (%s)", command.ConfigURL, command.ConfigMethod))
		remoteCache = &config.RemoteCache{}
		cfg, _, err = config.LoadFromURLCached(remoteReq, remoteCache, 0) // force first fetch
		if err != nil {
			return fmt.Errorf("remote config: %w", err)
		}
		a.logger.Information("Remote configuration loaded and validated successfully")
	} else {
		// Auto-create config if it doesn't exist
		if _, err := os.Stat(command.ConfigPath); os.IsNotExist(err) {
			a.logger.Information(fmt.Sprintf("Config file %q not found — generating default config", command.ConfigPath))
		}

		// §21.1 steps 1-2: load and validate config
		cfg, err = config.Load(command.ConfigPath)
		if err != nil {
			return err
		}
	}

	// Apply CLI/env schedule override before anything reads it
	if command.OverrideSchedule != "" {
		a.logger.Information(fmt.Sprintf("Overriding schedule: %q → %q",
			cfg.Settings.Runtime.Schedule, command.OverrideSchedule))
		cfg.Settings.Runtime.Schedule = command.OverrideSchedule
	}

	// §21.1 step 3: initialize centralized logger
	a.logger.SetLevel(logging.ParseLevel(cfg.Settings.Runtime.LogLevel))

	// Set up rotating log file.
	// Priority: LOG_PATH env var → state file's directory → binary's directory.
	logDir := os.Getenv("LOG_PATH")
	if logDir == "" {
		if storePath := cfg.Settings.Runtime.StatePath; storePath != "" {
			logDir = filepath.Dir(storePath)
		}
	}
	if logDir == "" {
		logDir = binaryDir()
	}
	exeName := filepath.Base(executablePath())
	logFileWriter, err := logging.NewRotatingFileWriter(logDir, exeName)
	if err != nil {
		a.logger.Warning(fmt.Sprintf("Failed to initialize log file rotation: %s", err))
	} else {
		a.logger.AttachFileWriter(logFileWriter)
		defer a.logger.CloseFileWriter()
		a.logger.Information(fmt.Sprintf("Log file rotation enabled: dir=%s, prefix=%s, maxSize=10MB, maxFiles=3", logDir, exeName))
	}

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
		// Re-fetch remote config if TTL has expired
		if remoteCache != nil && command.ConfigURL != "" {
			ttl := resolveConfigTTL(command.ConfigTTL, cfg.Settings.Runtime.Remote)
			newCfg, changed, fetchErr := config.LoadFromURLCached(remoteReq, remoteCache, ttl)
			if fetchErr != nil {
				a.logger.Warning(fmt.Sprintf("Remote config re-fetch failed (using cached): %s", fetchErr))
			} else if changed {
				a.logger.Information("Remote configuration updated — reloading providers")
				if command.OverrideSchedule != "" {
					newCfg.Settings.Runtime.Schedule = command.OverrideSchedule
				}
				a.logger.SetLevel(logging.ParseLevel(newCfg.Settings.Runtime.LogLevel))
				rs.mu.Lock()
				rs.cfg = newCfg
				rs.providers = a.buildProviderMap(newCfg)
				rs.mu.Unlock()
			}
		}

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

		// Phase 2: expand containerRecords templates against discovered containers
		if len(currentCfg.ContainerRecords) > 0 && snap.ContainerDetector != nil {
			containerTemplates := reconcile.ExpandContainerRecords(ctx, a.logger, snap.ContainerDetector, &currentCfg)
			if len(containerTemplates) > 0 {
				a.logger.Information(fmt.Sprintf("Container discovery: generated %d record(s) from %d template(s)",
					len(containerTemplates), len(currentCfg.ContainerRecords)))
				mergedRecords = append(mergedRecords, containerTemplates...)
			}
		}

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

	// Start health check HTTP server
	healthSrv := &http.Server{Addr: ":8000"}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","version":"%s"}`, Version)
	})
	go func() {
		a.logger.Information("Health check endpoint listening on :8000/health")
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Warning(fmt.Sprintf("Health check server error: %s", err))
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		healthSrv.Shutdown(shutdownCtx)
	}()

	// Start config file watcher
	fw := watcher.New(command.ConfigPath, a.logger)
	if err := fw.Init(); err != nil {
		a.logger.Warning(fmt.Sprintf("Config watcher init failed (continuing without): %s", err))
	}
	configChanged := fw.Watch(ctx)

	// Launch goroutine that reloads config when the file changes.
	go a.watchConfigReload(ctx, command, rs, configChanged)

	loc, err := scheduler.ResolveTimezone(cfg.Settings.Runtime.Timezone)
	if err != nil {
		return fmt.Errorf("timezone resolution failed: %w", err)
	}
	a.logger.Information(fmt.Sprintf("Scheduler timezone: %s", loc))

	sched, err := scheduler.New(a.logger, scheduler.Config{
		Schedule: cfg.Settings.Runtime.Schedule,
		Jitter:   cfg.Settings.Runtime.Jitter,
		Location: loc,
	}, reconcileOnce)
	if err != nil {
		return fmt.Errorf("scheduler init failed: %w", err)
	}

	humanSchedule := scheduler.HumanReadableSchedule(cfg.Settings.Runtime.Schedule)
	if humanSchedule != "" {
		a.logger.Information(fmt.Sprintf("Starting scheduler (schedule=%q [%s], jitter=%s, tz=%s)",
			cfg.Settings.Runtime.Schedule, humanSchedule, cfg.Settings.Runtime.Jitter, loc))
	} else {
		a.logger.Information(fmt.Sprintf("Starting scheduler (schedule=%q, jitter=%s, tz=%s)",
			cfg.Settings.Runtime.Schedule, cfg.Settings.Runtime.Jitter, loc))
	}
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
			if command.OverrideSchedule != "" {
				newCfg.Settings.Runtime.Schedule = command.OverrideSchedule
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

// executablePath returns the path to the running binary, falling back to
// os.Args[0] if os.Executable fails.
func executablePath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return os.Args[0]
}

// binaryDir returns the directory containing the running binary.
func binaryDir() string {
	return filepath.Dir(executablePath())
}