package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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
	OverrideSchedule string // "" = not set; cron expression overrides config
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
	schedule := fs.String("schedule", "", "Override cron schedule (6-field, seconds enabled).")
	if err := fs.Parse(args); err != nil {
		return Command{}, err
	}

	// Environment variable fallback: RECONCILE_SCHEDULE (cron expression)
	overrideSchedule := *schedule
	if overrideSchedule == "" {
		if envVal := os.Getenv("RECONCILE_SCHEDULE"); envVal != "" {
			overrideSchedule = envVal
		}
	}

	return Command{
		Kind:             CommandRun,
		ConfigPath:       *configPath,
		OverrideState:    *statePath,
		NodeID:           *nodeID,
		Once:             *once,
		OverrideSchedule: overrideSchedule,
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