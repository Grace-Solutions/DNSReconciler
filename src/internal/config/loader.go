package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

var (
	errUnsupportedVersion = errors.New("unsupported config version")
	errInvalidRecordType  = errors.New("record type must be A or AAAA in v1")
)

// Load reads and validates the config from path. If the file does not exist a
// sensible default config is written to that path first so the user has a
// working starting point.
func Load(path string) (Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if wErr := WriteDefault(path); wErr != nil {
			return Config{}, fmt.Errorf("auto-create default config: %w", wErr)
		}
		// Signal that we created it — caller can log this.
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	cfg.ApplyBuiltInDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("%w: %d", errUnsupportedVersion, c.Version)
	}
	if !isAllowed(strings.ToLower(c.Runtime.LogLevel), "", "trace", "debug", "information", "warning", "error", "critical") {
		return fmt.Errorf("runtime.logLevel %q is unsupported", c.Runtime.LogLevel)
	}
	if c.Defaults.Ownership != "" && !isAllowed(c.Defaults.Ownership, "perNode", "singleton", "manual", "disabled") {
		return fmt.Errorf("defaults.ownership %q is unsupported", c.Defaults.Ownership)
	}
	if err := validateSources("network.addressSources", c.Network.AddressSources); err != nil {
		return err
	}
	seenIDs := map[string]struct{}{}
	for _, record := range c.Records {
		if record.ID == "" {
			return errors.New("records[].id is required")
		}
		if _, exists := seenIDs[record.ID]; exists {
			return fmt.Errorf("duplicate record id %q", record.ID)
		}
		seenIDs[record.ID] = struct{}{}
		if record.Provider == "" || record.Zone == "" || record.Type == "" || record.Name == "" || record.Content == "" {
			return fmt.Errorf("record %q must define provider, zone, type, name, and content", record.ID)
		}
		if record.Type != "A" && record.Type != "AAAA" {
			return fmt.Errorf("record %q: %w", record.ID, errInvalidRecordType)
		}
		if record.Ownership != "" && !isAllowed(record.Ownership, "perNode", "singleton", "manual", "disabled") {
			return fmt.Errorf("record %q has unsupported ownership %q", record.ID, record.Ownership)
		}
		if record.IPFamily != "" && !isAllowed(strings.ToLower(record.IPFamily), "ipv4", "ipv6", "dual") {
			return fmt.Errorf("record %q has unsupported ipFamily %q", record.ID, record.IPFamily)
		}
		if record.AddressSelection != nil {
			if err := validateSources("records["+record.ID+"].addressSelection.sources", record.AddressSelection.Sources); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSources(path string, sources []AddressSource) error {
	seenPriorities := map[int]struct{}{}
	for _, source := range sources {
		if source.Priority < 1 || source.Priority > 4 {
			return fmt.Errorf("%s priority %d is invalid; expected 1 through 4", path, source.Priority)
		}
		if source.Type == "" {
			return fmt.Errorf("%s contains a source with no type", path)
		}
		if _, exists := seenPriorities[source.Priority]; exists {
			return fmt.Errorf("%s contains duplicate priority %d", path, source.Priority)
		}
		seenPriorities[source.Priority] = struct{}{}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})
	return nil
}

func isAllowed(value string, supported ...string) bool {
	for _, candidate := range supported {
		if value == candidate {
			return true
		}
	}
	return false
}