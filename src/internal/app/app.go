package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/address"
	"github.com/gracesolutions/dns-automatic-updater/internal/cleanup"
	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/cloudflare"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/powerdns"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider/technitium"
	"github.com/gracesolutions/dns-automatic-updater/internal/reconcile"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
	"github.com/gracesolutions/dns-automatic-updater/internal/scheduler"
	"github.com/gracesolutions/dns-automatic-updater/internal/service"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
)

const version = "dev"

type Application struct {
	logger         *logging.Logger
	serviceManager service.Manager
	stdout         io.Writer
	stderr         io.Writer
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
	// §21.1 steps 1-2: load and validate config
	cfg, err := config.Load(command.ConfigPath)
	if err != nil {
		return err
	}

	// §21.1 step 3: initialize centralized logger
	a.logger.SetLevel(logging.ParseLevel(cfg.Runtime.LogLevel))

	// §21.1 step 4: load local state
	storePath := cfg.Runtime.StatePath
	if command.OverrideState != "" {
		storePath = command.OverrideState
	}
	store := state.JSONStore{Path: storePath}

	// Build providers once (§9)
	providers := a.buildProviderMap(cfg)

	// reconcileOnce performs a single reconciliation pass (§21.1 steps 5-10).
	reconcileOnce := func(ctx context.Context) error {
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
		mergedRecords := config.MergeAllDefaults(&cfg)

		// §21.1 steps 6, 8-9: resolve addresses, expand, reconcile
		addrResolver := address.NewDefaultResolver(a.logger)

		reconciler := reconcile.Reconciler{
			Logger:          a.logger,
			Providers:       providers,
			AddressResolver: addrResolver,
			Snapshot:        snap,
			GlobalSources:   cfg.Network.AddressSources,
			DryRun:          cfg.Runtime.DryRun,
		}

		stats, _ := reconciler.ReconcileAll(ctx, mergedRecords, &st)

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

	sched := scheduler.New(a.logger, scheduler.Config{
		IntervalSeconds: cfg.Runtime.ReconcileIntervalSeconds,
		JitterPercent:   10,
	}, reconcileOnce)

	a.logger.Information(fmt.Sprintf("Starting scheduler (interval=%ds)", cfg.Runtime.ReconcileIntervalSeconds))
	err = sched.Run(ctx)
	if err != nil && err != context.Canceled {
		return err
	}
	a.logger.Information("Scheduler stopped gracefully")

	// §26.1: cleanup on graceful shutdown
	if cfg.Runtime.CleanupOnShutdown {
		a.logger.Information("CleanupOnShutdown is enabled — deleting owned records")
		cleaner := cleanup.Cleaner{
			Logger:    a.logger,
			Providers: providers,
			Store:     &store,
		}
		// Use a fresh context since the signal context is cancelled
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := cleaner.Run(cleanupCtx); err != nil {
			a.logger.Error(fmt.Sprintf("Cleanup failed: %s", err))
		}
	}

	return nil
}

// buildProviderMap creates provider instances from config using the provider registry (§9).
func (a Application) buildProviderMap(cfg config.Config) map[string]core.Provider {
	registry := provider.NewRegistry()
	registry.Register("cloudflare", cloudflare.New)
	registry.Register("technitium", technitium.New)
	registry.Register("powerdns", powerdns.New)

	// Collect unique provider names referenced by records
	needed := map[string]bool{}
	for _, rec := range cfg.Records {
		needed[rec.Provider] = true
	}

	providers := map[string]core.Provider{}
	for name := range needed {
		provCfg := cfg.ProviderDefaults[name]
		if provCfg == nil {
			provCfg = map[string]any{}
		}
		p, err := registry.Build(name, provCfg, a.logger)
		if err != nil {
			a.logger.Error(fmt.Sprintf("Failed to initialize provider %q: %s", name, err))
			continue
		}
		providers[name] = p
		a.logger.Information(fmt.Sprintf("Provider %q initialized successfully", name))
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
	case service.ActionRemove:
		return a.serviceManager.Remove(ctx, options)
	case service.ActionStart:
		return a.serviceManager.Start(ctx, options)
	case service.ActionStop:
		return a.serviceManager.Stop(ctx, options)
	default:
		return fmt.Errorf("unsupported service action %q", command.ServiceAction)
	}
}