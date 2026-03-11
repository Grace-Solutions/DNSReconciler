package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
)

// Config is the top-level configuration structure.
// The schema uses a providers array (instance-based) instead of a type-keyed map,
// allowing multiple instances of the same provider type (e.g. two Cloudflare accounts).
type Config struct {
	Settings  SettingsConfig   `json:"settings"`
	Providers []ProviderEntry  `json:"providers"`
	Records   []RecordTemplate `json:"records"`
}

// SettingsConfig wraps runtime and network configuration under a single key.
type SettingsConfig struct {
	Runtime RuntimeConfig `json:"runtime"`
	Network NetworkConfig `json:"network"`
}

// RuntimeConfig holds scheduler, state, and logging settings.
type RuntimeConfig struct {
	ReconcileIntervalSeconds int    `json:"reconcileIntervalSeconds"`
	StatePath                string `json:"statePath"`
	CleanupOnShutdown        bool   `json:"cleanupOnShutdown"`
	LogLevel                 string `json:"logLevel"`
	DryRun                   bool   `json:"dryRun"`
}

// NetworkConfig holds address source configuration.
type NetworkConfig struct {
	AddressSources []AddressSource `json:"addressSources"`
}

// ProviderEntry defines a single provider instance with its credentials and defaults.
// The ID can be a UUIDv4 or a friendly name; it must be unique across all providers.
// RawConfig captures all JSON fields so provider factories can read credentials.
type ProviderEntry struct {
	ID           string          `json:"id"`
	FriendlyName string          `json:"friendlyName,omitempty"`
	Type         string          `json:"type"`
	Enabled      *bool           `json:"enabled,omitempty"`
	Defaults     RecordDefaults  `json:"defaults"`
	RawConfig    map[string]any  `json:"-"` // populated during unmarshal
}

// UnmarshalJSON implements custom unmarshaling for ProviderEntry.
// It decodes structured fields into the struct and captures all raw JSON
// key-value pairs into RawConfig for credential resolution by provider factories.
func (p *ProviderEntry) UnmarshalJSON(data []byte) error {
	// Decode into a type alias to avoid recursion.
	type plain ProviderEntry
	var s plain
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*p = ProviderEntry(s)

	// Capture full raw map for provider factory credential access.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.RawConfig = raw
	return nil
}

// IsEnabled returns whether the provider instance is active.
func (p *ProviderEntry) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

// RecordDefaults holds default values that can be set at the provider level
// and inherited by records referencing that provider.
type RecordDefaults struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Ownership string `json:"ownership,omitempty"`
	TTL       *int   `json:"ttl,omitempty"`
	Proxied   *bool  `json:"proxied,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Tags      []Tag  `json:"tags,omitempty"`
}

// RecordTemplate defines a DNS record to be reconciled.
// ProviderID links to a ProviderEntry by its ID or FriendlyName.
type RecordTemplate struct {
	ID               string            `json:"id"`
	Enabled          *bool             `json:"enabled,omitempty"`
	ProviderID       string            `json:"providerId"`
	Ownership        string            `json:"ownership,omitempty"`
	Zone             string            `json:"zone"`
	Type             string            `json:"type"`
	Name             string            `json:"name"`
	Content          string            `json:"content"`
	TTL              *int              `json:"ttl,omitempty"`
	Proxied          *bool             `json:"proxied,omitempty"`
	Comment          string            `json:"comment,omitempty"`
	Tags             []Tag             `json:"tags,omitempty"`
	MatchLabels      map[string]string `json:"matchLabels,omitempty"`
	AddressSelection *AddressSelection `json:"addressSelection,omitempty"`
	IPFamily         string            `json:"ipFamily,omitempty"`
}

type AddressSelection struct {
	UseGlobalDefaults *bool           `json:"useGlobalDefaults,omitempty"`
	Sources           []AddressSource `json:"sources,omitempty"`
}

type AddressSource struct {
	Priority      int      `json:"priority"`
	Type          string   `json:"type"`
	Enabled       bool     `json:"enabled"`
	InterfaceName string   `json:"interfaceName,omitempty"`
	AllowRanges   []string `json:"allowRanges,omitempty"`
	DenyRanges    []string `json:"denyRanges,omitempty"`
	AddressFamily string   `json:"addressFamily,omitempty"`
	ExplicitValue string   `json:"explicitValue,omitempty"`
}

type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FindProvider looks up a provider entry by ID or FriendlyName.
// Returns nil if not found.
func (c *Config) FindProvider(idOrName string) *ProviderEntry {
	for i := range c.Providers {
		if c.Providers[i].ID == idOrName || c.Providers[i].FriendlyName == idOrName {
			return &c.Providers[i]
		}
	}
	return nil
}

// ApplyBuiltInDefaults fills in zero-valued runtime settings with sensible defaults.
func (c *Config) ApplyBuiltInDefaults() {
	if c.Settings.Runtime.ReconcileIntervalSeconds == 0 {
		c.Settings.Runtime.ReconcileIntervalSeconds = 120
	}
	if c.Settings.Runtime.StatePath == "" {
		c.Settings.Runtime.StatePath = "./state.json"
	}
	if c.Settings.Runtime.LogLevel == "" {
		c.Settings.Runtime.LogLevel = "Information"
	}
	// Per-provider defaults are applied during MergeDefaults, not here.
}

// GenerateUUID returns a new UUIDv4 string using crypto/rand.
func GenerateUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}