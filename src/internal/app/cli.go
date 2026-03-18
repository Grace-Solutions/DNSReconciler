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
	ConfigTTL        string // Go duration for remote config re-fetch interval (e.g. "30m", "1h")
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
	configTTL := fs.String("config-ttl", "", "Re-fetch interval for remote config (e.g. 30m, 1h). Default: 1h.")
	if err := fs.Parse(args); err != nil {
		return Command{}, err
	}

	// Environment variable fallbacks — CLI flags take priority over env vars.
	cfgPath := *configPath
	if cfgPath == "./config.json" {
		if envVal := os.Getenv("CONFIG_PATH"); envVal != "" {
			cfgPath = envVal
		}
	}
	stPath := *statePath
	if stPath == "" {
		if envVal := os.Getenv("STATE_PATH"); envVal != "" {
			stPath = envVal
		}
	}
	nID := *nodeID
	if nID == "" {
		if envVal := os.Getenv("NODE_ID"); envVal != "" {
			nID = envVal
		}
	}
	runOnce := *once
	if !runOnce {
		if envVal := os.Getenv("ONCE"); envVal == "true" || envVal == "1" {
			runOnce = true
		}
	}
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
	cfgMethod := *configMethod
	if cfgMethod == "GET" {
		if envVal := os.Getenv("CONFIG_METHOD"); envVal != "" {
			cfgMethod = envVal
		}
	}
	cfgTTL := *configTTL
	if cfgTTL == "" {
		if envVal := os.Getenv("CONFIG_TTL"); envVal != "" {
			cfgTTL = envVal
		}
	}

	return Command{
		Kind:             CommandRun,
		ConfigPath:       cfgPath,
		OverrideState:    stPath,
		NodeID:           nID,
		Once:             runOnce,
		OverrideSchedule: overrideSchedule,
		ConfigURL:        cfgURL,
		ConfigHeader:     cfgHeader,
		ConfigToken:      cfgToken,
		ConfigMethod:     cfgMethod,
		ConfigTTL:        cfgTTL,
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