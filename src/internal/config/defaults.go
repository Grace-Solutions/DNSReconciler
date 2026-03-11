package config

// MergeDefaults applies the §18 inheritance chain to a record template:
// built-in → provider defaults → global defaults → per-record values.
// The result is a fully-resolved copy; the original is not mutated.
func MergeDefaults(record RecordTemplate, globalDefaults RecordDefaults, providerDefaults map[string]map[string]any) RecordTemplate {
	merged := record

	// 1. built-in defaults are already applied to globalDefaults via ApplyBuiltInDefaults.
	// 2. provider defaults — apply provider-level zone if available and record has none.
	if pd, ok := providerDefaults[record.Provider]; ok {
		if merged.Zone == "" {
			if z, ok := pd["zone"].(string); ok {
				merged.Zone = z
			}
		}
	}

	// 3. global defaults — fill any unset fields from globalDefaults.
	if merged.Enabled == nil {
		merged.Enabled = globalDefaults.Enabled
	}
	if merged.Ownership == "" {
		merged.Ownership = globalDefaults.Ownership
	}
	if merged.TTL == nil {
		merged.TTL = globalDefaults.TTL
	}
	if merged.Proxied == nil {
		merged.Proxied = globalDefaults.Proxied
	}
	if merged.Comment == "" {
		merged.Comment = globalDefaults.Comment
	}
	if len(merged.Tags) == 0 && len(globalDefaults.Tags) > 0 {
		merged.Tags = make([]Tag, len(globalDefaults.Tags))
		copy(merged.Tags, globalDefaults.Tags)
	}

	// 4. per-record values already take precedence because they were set first.
	return merged
}

// MergeAllDefaults applies MergeDefaults to every record in the config,
// returning a new slice of fully-resolved templates.
func MergeAllDefaults(cfg *Config) []RecordTemplate {
	result := make([]RecordTemplate, len(cfg.Records))
	for i, rec := range cfg.Records {
		result[i] = MergeDefaults(rec, cfg.Defaults, cfg.ProviderDefaults)
	}
	return result
}

