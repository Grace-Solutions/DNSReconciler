package config

type Config struct {
	Version          int                           `json:"version"`
	ProviderDefaults map[string]map[string]any    `json:"providerDefaults"`
	Runtime          RuntimeConfig                 `json:"runtime"`
	Network          NetworkConfig                 `json:"network"`
	Defaults         RecordDefaults                `json:"defaults"`
	Records          []RecordTemplate              `json:"records"`
}

type RuntimeConfig struct {
	ReconcileIntervalSeconds int    `json:"reconcileIntervalSeconds"`
	StatePath                string `json:"statePath"`
	CleanupOnShutdown        bool   `json:"cleanupOnShutdown"`
	LogLevel                 string `json:"logLevel"`
	DryRun                   bool   `json:"dryRun"`
}

type NetworkConfig struct {
	AddressSources []AddressSource `json:"addressSources"`
}

type RecordDefaults struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Ownership string `json:"ownership,omitempty"`
	TTL       *int   `json:"ttl,omitempty"`
	Proxied   *bool  `json:"proxied,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Tags      []Tag  `json:"tags,omitempty"`
}

type RecordTemplate struct {
	ID               string                `json:"id"`
	Enabled          *bool                 `json:"enabled,omitempty"`
	Provider         string                `json:"provider"`
	Ownership        string                `json:"ownership,omitempty"`
	Zone             string                `json:"zone"`
	Type             string                `json:"type"`
	Name             string                `json:"name"`
	Content          string                `json:"content"`
	TTL              *int                  `json:"ttl,omitempty"`
	Proxied          *bool                 `json:"proxied,omitempty"`
	Comment          string                `json:"comment,omitempty"`
	Tags             []Tag                 `json:"tags,omitempty"`
	MatchLabels      map[string]string     `json:"matchLabels,omitempty"`
	AddressSelection *AddressSelection     `json:"addressSelection,omitempty"`
	IPFamily         string                `json:"ipFamily,omitempty"`
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

func (c *Config) ApplyBuiltInDefaults() {
	if c.ProviderDefaults == nil {
		c.ProviderDefaults = map[string]map[string]any{}
	}
	if c.Runtime.ReconcileIntervalSeconds == 0 {
		c.Runtime.ReconcileIntervalSeconds = 120
	}
	if c.Runtime.StatePath == "" {
		c.Runtime.StatePath = "./state.json"
	}
	if c.Runtime.LogLevel == "" {
		c.Runtime.LogLevel = "Information"
	}
	if c.Defaults.Enabled == nil {
		value := true
		c.Defaults.Enabled = &value
	}
	if c.Defaults.Ownership == "" {
		c.Defaults.Ownership = "perNode"
	}
	if c.Defaults.TTL == nil {
		value := 120
		c.Defaults.TTL = &value
	}
	if c.Defaults.Proxied == nil {
		value := false
		c.Defaults.Proxied = &value
	}
}