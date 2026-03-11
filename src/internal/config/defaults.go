package config

func boolp(v bool) *bool { return &v }
func intp(v int) *int    { return &v }

// MergeDefaults applies the inheritance chain to a record template:
// built-in defaults → provider-level values → per-record values.
// The result is a fully-resolved copy; the original is not mutated.
func MergeDefaults(record RecordTemplate, prov *ProviderEntry) RecordTemplate {
	merged := record

	// Priority (highest to lowest): per-record > provider > built-in.
	// Since we only fill *unset* fields, apply in order of decreasing priority
	// so higher-priority values are set first and lower-priority calls skip them.

	// 1. Per-record values are already on `merged` (highest priority).

	// 2. Provider-level values fill remaining gaps.
	if prov != nil {
		if merged.Zone == "" {
			merged.Zone = prov.Zone
		}
		if merged.TTL == nil {
			merged.TTL = prov.TTL
		}
		if merged.Proxied == nil {
			merged.Proxied = prov.Proxied
		}
		if merged.Comment == "" {
			merged.Comment = prov.Comment
		}
		if len(merged.Tags) == 0 && len(prov.Tags) > 0 {
			merged.Tags = make([]Tag, len(prov.Tags))
			copy(merged.Tags, prov.Tags)
		}
	}

	// 3. Built-in defaults fill anything still unset (lowest priority).
	if merged.Enabled == nil {
		merged.Enabled = boolp(true)
	}
	if merged.Ownership == "" {
		merged.Ownership = "perNode"
	}
	if merged.TTL == nil {
		merged.TTL = intp(120)
	}
	if merged.Proxied == nil {
		merged.Proxied = boolp(false)
	}

	return merged
}

// MergeAllDefaults applies MergeDefaults to every record in the config,
// resolving each record's provider to obtain provider-level defaults.
func MergeAllDefaults(cfg *Config) []RecordTemplate {
	result := make([]RecordTemplate, len(cfg.Records))
	for i, rec := range cfg.Records {
		prov := cfg.FindProvider(rec.ProviderID)
		result[i] = MergeDefaults(rec, prov)
	}
	return result
}

