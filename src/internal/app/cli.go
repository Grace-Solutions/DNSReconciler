package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/gracesolutions/dns-automatic-updater/internal/service"
)

type CommandKind string

const (
	CommandRun     CommandKind = "run"
	CommandService CommandKind = "service"
	CommandVersion CommandKind = "version"
)

type Command struct {
	Kind             CommandKind
	ConfigPath       string
	OverrideState    string
	NodeID           string
	Once             bool
	OverrideInterval int // 0 = not set; positive value overrides config
	ServiceAction    service.Action
	ServiceName      string
}

func Parse(args []string) (Command, error) {
	if len(args) > 0 {
		switch args[0] {
		case "service":
			return parseService(args[1:])
		case "version":
			return Command{Kind: CommandVersion}, nil
		}
	}
	return parseRun(args)
}

func parseRun(args []string) (Command, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "./config.json", "Path to the JSON configuration file.")
	statePath := fs.String("state", "", "Override the configured state file path.")
	nodeID := fs.String("node-id", "", "Explicit node identity (§11.4).")
	once := fs.Bool("once", false, "Run a single reconciliation pass and exit (§25).")
	interval := fs.Int("interval", 0, "Override reconcile interval in seconds.")
	if err := fs.Parse(args); err != nil {
		return Command{}, err
	}

	// Environment variable fallback: RECONCILE_INTERVAL_SECONDS
	overrideInterval := *interval
	if overrideInterval == 0 {
		if envVal := os.Getenv("RECONCILE_INTERVAL_SECONDS"); envVal != "" {
			if parsed, err := strconv.Atoi(envVal); err == nil && parsed > 0 {
				overrideInterval = parsed
			}
		}
	}

	return Command{
		Kind:             CommandRun,
		ConfigPath:       *configPath,
		OverrideState:    *statePath,
		NodeID:           *nodeID,
		Once:             *once,
		OverrideInterval: overrideInterval,
	}, nil
}

func parseService(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, errors.New("service action is required: install, uninstall, start, stop, or init")
	}
	action := service.Action(args[0])
	switch action {
	case service.ActionInstall, service.ActionUninstall, service.ActionStart, service.ActionStop, service.ActionInit:
	default:
		return Command{}, fmt.Errorf("unsupported service action %q", args[0])
	}
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	serviceName := fs.String("name", "dnsreconciler", "Service name.")
	if err := fs.Parse(args[1:]); err != nil {
		return Command{}, err
	}
	return Command{Kind: CommandService, ServiceAction: action, ServiceName: *serviceName}, nil
}