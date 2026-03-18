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
	Settings         SettingsConfig            `json:"settings"`
	Providers        []ProviderEntry           `json:"providers"`
	Records          []RecordTemplate          `json:"records"`
	ContainerRecords []ContainerRecordTemplate `json:"containerRecords,omitempty"`
}

// SettingsConfig wraps runtime and network configuration under a single key.
type SettingsConfig struct {
	Runtime RuntimeConfig `json:"runtime"`
	Network NetworkConfig `json:"network"`
}

// RuntimeConfig holds scheduler, state, and logging settings.
type RuntimeConfig struct {
	Schedule          string        `json:"schedule"`
	Jitter            string        `json:"jitter,omitempty"`
	Timezone          string        `json:"timezone,omitempty"`
	StatePath         string        `json:"statePath"`
	CleanupOnShutdown bool          `json:"cleanupOnShutdown"`
	LogLevel          string        `json:"logLevel"`
	DryRun            bool          `json:"dryRun"`
	Remote            *RemoteConfig `json:"remote,omitempty"`
}

// RemoteConfig holds settings for remote (URL-based) configuration fetching.
type RemoteConfig struct {
	TTL string `json:"ttl,omitempty"` // Go duration string (e.g. "30m", "1h"). Default: "1h".
}

// NetworkConfig holds address source configuration.
type NetworkConfig struct {
	AddressSources []AddressSource `json:"addressSources"`
}

// ProviderEntry defines a single provider instance with its credentials and
// inheritable record defaults. Default values for TTL, Proxied, Comment, Tags,
// and Zone live directly on the provider — no nested "defaults" wrapper.
// RawConfig captures all JSON fields so provider factories can read credentials.
type ProviderEntry struct {
	ID           string         `json:"providerId"`
	FriendlyName string         `json:"friendlyName,omitempty"`
	Type         string         `json:"type"`
	Enabled      *bool          `json:"enabled,omitempty"`
	Zone         string         `json:"zone,omitempty"`
	TTL          *int           `json:"ttl,omitempty"`
	Proxied      *bool          `json:"proxied,omitempty"`
	Comment      string         `json:"comment,omitempty"`
	Tags         []Tag          `json:"tags,omitempty"`
	RawConfig    map[string]any `json:"-"` // populated during unmarshal
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

// RecordTemplate defines a DNS record to be reconciled.
// ProviderID links to a ProviderEntry by its UUID (providerId).
// Zone is optional — inherited from the provider if not set.
type RecordTemplate struct {
	ProviderID       string            `json:"providerId"`
	RecordID         string            `json:"recordId"`
	Enabled          *bool             `json:"enabled,omitempty"`
	Type             string            `json:"type"`
	Name             string            `json:"name"`
	Content          string            `json:"content"`
	Zone             string            `json:"zone,omitempty"`
	TTL              *int              `json:"ttl,omitempty"`
	Proxied          *bool             `json:"proxied,omitempty"`
	Comment          string            `json:"comment,omitempty"`
	Tags             []Tag             `json:"tags,omitempty"`
	Ownership        string            `json:"ownership,omitempty"`
	MatchLabels      map[string]string `json:"matchLabels,omitempty"`
	AddressSelection *AddressSelection `json:"addressSelection,omitempty"`
	IPFamily         string            `json:"ipFamily,omitempty"`

	// ContainerMeta is set only for records generated from containerRecords
	// templates. It carries the discovered container's metadata for variable
	// expansion. Not serialized to JSON — populated at runtime.
	ContainerMeta *ContainerMeta `json:"-"`
}

// ContainerMeta holds container-specific metadata attached to a generated
// RecordTemplate so the reconciler can build the container-aware expansion context.
type ContainerMeta struct {
	ContainerName     string
	ContainerHostname string // hostname from container inspect
	ContainerID       string // short ID (12 chars)
	ContainerIP       string // IP on the routable network
	ContainerImage    string
	Labels            map[string]string
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

// ContainerRecordTemplate defines a DNS record template that is expanded once
// per discovered container on a routable network (IPVLAN or MACVLAN). The
// template fields support the same variable expansion as RecordTemplate plus
// container-specific variables: CONTAINER_NAME, CONTAINER_ID, CONTAINER_IP,
// CONTAINER_IMAGE, and label lookups via LABEL:<key>.
//
// Ownership tags (nodeId, containerName, containerId) are auto-injected at
// expansion time so providers with structured tag support can match ownership
// without being constrained by comment length limits.
type ContainerRecordTemplate struct {
	Description string   `json:"description,omitempty"`
	ProviderID  string   `json:"providerId"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Content     string   `json:"content"`
	Zone        string   `json:"zone,omitempty"`
	TTL         *int     `json:"ttl,omitempty"`
	Proxied     *bool    `json:"proxied,omitempty"`
	Comment     string   `json:"comment,omitempty"`
	Tags        []Tag    `json:"tags,omitempty"`
	Ownership   string   `json:"ownership,omitempty"`
	Include     []string `json:"include,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	MatchFields []string `json:"matchFields,omitempty"`
}

// IsEnabled returns whether this container record template is active.
func (c *ContainerRecordTemplate) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// FindProvider looks up a provider entry by its UUID (providerId).
// The friendlyName is for display only and is not used for lookup.
// Returns nil if not found.
func (c *Config) FindProvider(id string) *ProviderEntry {
	for i := range c.Providers {
		if c.Providers[i].ID == id {
			return &c.Providers[i]
		}
	}
	return nil
}

// ApplyBuiltInDefaults fills in zero-valued runtime settings with sensible defaults.
func (c *Config) ApplyBuiltInDefaults() {
	if c.Settings.Runtime.Schedule == "" {
		c.Settings.Runtime.Schedule = "0 0 */4 * * *"
	}
	if c.Settings.Runtime.Jitter == "" {
		c.Settings.Runtime.Jitter = "auto"
	}
	if c.Settings.Runtime.Timezone == "" {
		c.Settings.Runtime.Timezone = "UTC"
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