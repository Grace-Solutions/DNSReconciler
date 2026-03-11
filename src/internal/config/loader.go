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
	errInvalidRecordType = errors.New("record type must be A or AAAA")
)

// Load reads and validates the config from path. If the file does not exist a
// sensible default config is written to that path first so the user has a
// working starting point.
func Load(path string) (Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if wErr := WriteDefault(path); wErr != nil {
			return Config{}, fmt.Errorf("auto-create default config: %w", wErr)
		}
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

// Validate checks the config for structural and semantic correctness.
func (c Config) Validate() error {
	if !isAllowed(strings.ToLower(c.Settings.Runtime.LogLevel), "", "trace", "debug", "information", "warning", "error", "critical") {
		return fmt.Errorf("settings.runtime.logLevel %q is unsupported", c.Settings.Runtime.LogLevel)
	}
	if err := validateSources("settings.network.addressSources", c.Settings.Network.AddressSources); err != nil {
		return err
	}

	// Validate providers
	seenProvIDs := map[string]struct{}{}
	seenFriendly := map[string]struct{}{}
	for _, prov := range c.Providers {
		if prov.ID == "" {
			return errors.New("providers[].id is required")
		}
		if _, exists := seenProvIDs[prov.ID]; exists {
			return fmt.Errorf("duplicate provider id %q", prov.ID)
		}
		seenProvIDs[prov.ID] = struct{}{}
		if prov.FriendlyName != "" {
			if _, exists := seenFriendly[prov.FriendlyName]; exists {
				return fmt.Errorf("duplicate provider friendlyName %q", prov.FriendlyName)
			}
			seenFriendly[prov.FriendlyName] = struct{}{}
		}
		if prov.Type == "" {
			return fmt.Errorf("provider %q must define a type", prov.ID)
		}
	}

	// Validate records
	seenIDs := map[string]struct{}{}
	for i, record := range c.Records {
		if record.RecordID == "" {
			return errors.New("records[].recordId is required")
		}
		if _, exists := seenIDs[record.RecordID]; exists {
			return fmt.Errorf("duplicate record id %q", record.RecordID)
		}
		seenIDs[record.RecordID] = struct{}{}
		if record.ProviderID == "" || record.Type == "" || record.Name == "" || record.Content == "" {
			return fmt.Errorf("record %q must define providerId, type, name, and content", record.RecordID)
		}
		// Validate that referenced provider exists
		prov := c.FindProvider(record.ProviderID)
		if prov == nil {
			return fmt.Errorf("record %q references unknown provider %q", record.RecordID, record.ProviderID)
		}
		// Inherit zone from provider if not set on the record
		if record.Zone == "" {
			if prov.Zone == "" {
				return fmt.Errorf("record %q has no zone and provider %q defines no default zone", record.RecordID, record.ProviderID)
			}
			c.Records[i].Zone = prov.Zone
		}
		if record.Type != "A" && record.Type != "AAAA" {
			return fmt.Errorf("record %q: %w", record.RecordID, errInvalidRecordType)
		}
		if record.Ownership != "" && !isAllowed(record.Ownership, "perNode", "singleton", "manual", "disabled") {
			return fmt.Errorf("record %q has unsupported ownership %q", record.RecordID, record.Ownership)
		}
		if record.IPFamily != "" && !isAllowed(strings.ToLower(record.IPFamily), "ipv4", "ipv6", "dual") {
			return fmt.Errorf("record %q has unsupported ipFamily %q", record.RecordID, record.IPFamily)
		}
		if record.AddressSelection != nil {
			if err := validateSources("records["+record.RecordID+"].addressSelection.sources", record.AddressSelection.Sources); err != nil {
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