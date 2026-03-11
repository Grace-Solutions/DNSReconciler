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
	return Config{
		Settings: SettingsConfig{
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
		},
		Providers: []ProviderEntry{},
		Records:   []RecordTemplate{},
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

	data, err := json.MarshalIndent(DefaultConfig(), "", "    ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write default config %q: %w", path, err)
	}
	return nil
}

