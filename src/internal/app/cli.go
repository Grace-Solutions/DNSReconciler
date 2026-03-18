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
	ConfigURL        string // Remote URL to fetch configuration from
	ConfigHeader     string // HTTP header name for authentication (e.g. "Authorization")
	ConfigToken      string // HTTP header value / token
	ConfigMethod     string // HTTP method: GET (default) or POST
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
	configURL := fs.String("config-url", "", "Remote URL to fetch JSON configuration from.")
	configHeader := fs.String("config-header", "", "HTTP header name for remote config authentication (e.g. Authorization).")
	configToken := fs.String("config-token", "", "HTTP header value / bearer token for remote config.")
	configMethod := fs.String("config-method", "GET", "HTTP method for remote config fetch (GET or POST).")
	if err := fs.Parse(args); err != nil {
		return Command{}, err
	}

	// Environment variable fallbacks
	overrideSchedule := *schedule
	if overrideSchedule == "" {
		if envVal := os.Getenv("RECONCILE_SCHEDULE"); envVal != "" {
			overrideSchedule = envVal
		}
	}
	cfgURL := *configURL
	if cfgURL == "" {
		if envVal := os.Getenv("CONFIG_URL"); envVal != "" {
			cfgURL = envVal
		}
	}
	cfgHeader := *configHeader
	if cfgHeader == "" {
		if envVal := os.Getenv("CONFIG_HEADER"); envVal != "" {
			cfgHeader = envVal
		}
	}
	cfgToken := *configToken
	if cfgToken == "" {
		if envVal := os.Getenv("CONFIG_TOKEN"); envVal != "" {
			cfgToken = envVal
		}
	}

	return Command{
		Kind:             CommandRun,
		ConfigPath:       *configPath,
		OverrideState:    *statePath,
		NodeID:           *nodeID,
		Once:             *once,
		OverrideSchedule: overrideSchedule,
		ConfigURL:        cfgURL,
		ConfigHeader:     cfgHeader,
		ConfigToken:      cfgToken,
		ConfigMethod:     *configMethod,
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