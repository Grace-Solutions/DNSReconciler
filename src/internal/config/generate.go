package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultConfig returns a minimal, valid Config with sensible defaults that
// serves as a starting template when no config file exists.
func DefaultConfig() Config {
	enabled := true
	ttl := 120
	proxied := false

	return Config{
		Version:          1,
		ProviderDefaults: map[string]map[string]any{},
		Runtime: RuntimeConfig{
			ReconcileIntervalSeconds: 120,
			StatePath:                "./state.json",
			CleanupOnShutdown:        false,
			LogLevel:                 "Information",
			DryRun:                   false,
		},
		Network: NetworkConfig{
			AddressSources: []AddressSource{
				{Priority: 1, Type: "publicIPv4", Enabled: true},
				{Priority: 2, Type: "rfc1918IPv4", Enabled: true},
			},
		},
		Defaults: RecordDefaults{
			Enabled:   &enabled,
			Ownership: "perNode",
			TTL:       &ttl,
			Proxied:   &proxied,
			Comment:   "Managed by dnsreconciler on ${HOSTNAME}",
			Tags: []Tag{
				{Name: "managed-by", Value: "dnsreconciler"},
				{Name: "node-id", Value: "${NODE_ID}"},
			},
		},
		Records: []RecordTemplate{},
	}
}

// WriteDefault serialises a DefaultConfig to the given path, creating any
// parent directories as needed. It does NOT overwrite an existing file.
func WriteDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config file %q already exists", path)
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config directory %q: %w", dir, err)
		}
	}

	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write default config %q: %w", path, err)
	}
	return nil
}

