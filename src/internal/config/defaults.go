package config

// builtInDefaults returns the hard-coded default values for record properties
// when neither the provider nor the record specifies them.
var builtInDefaults = RecordDefaults{
	Enabled:   boolp(true),
	Ownership: "perNode",
	TTL:       intp(120),
	Proxied:   boolp(false),
}

func boolp(v bool) *bool { return &v }
func intp(v int) *int    { return &v }

// MergeDefaults applies the inheritance chain to a record template:
// built-in defaults → provider-level defaults → per-record values.
// The result is a fully-resolved copy; the original is not mutated.
func MergeDefaults(record RecordTemplate, providerDefaults RecordDefaults) RecordTemplate {
	merged := record

	// Priority (highest to lowest): per-record > provider-level > built-in.
	// Since applyDefaults only fills *unset* fields, we apply in order of
	// decreasing priority so higher-priority values are set first and
	// lower-priority calls skip fields already populated.

	// 1. Per-record values are already on `merged` (highest priority).
	// 2. Provider-level defaults fill remaining gaps.
	applyDefaults(&merged, providerDefaults)
	// 3. Built-in defaults fill anything still unset (lowest priority).
	applyDefaults(&merged, builtInDefaults)

	return merged
}

// applyDefaults fills in unset fields on the record from the given defaults.
func applyDefaults(rec *RecordTemplate, defs RecordDefaults) {
	if rec.Enabled == nil {
		rec.Enabled = defs.Enabled
	}
	if rec.Ownership == "" {
		rec.Ownership = defs.Ownership
	}
	if rec.TTL == nil {
		rec.TTL = defs.TTL
	}
	if rec.Proxied == nil {
		rec.Proxied = defs.Proxied
	}
	if rec.Comment == "" {
		rec.Comment = defs.Comment
	}
	if len(rec.Tags) == 0 && len(defs.Tags) > 0 {
		rec.Tags = make([]Tag, len(defs.Tags))
		copy(rec.Tags, defs.Tags)
	}
}

// MergeAllDefaults applies MergeDefaults to every record in the config,
// resolving each record's provider to obtain provider-level defaults.
func MergeAllDefaults(cfg *Config) []RecordTemplate {
	result := make([]RecordTemplate, len(cfg.Records))
	for i, rec := range cfg.Records {
		var pd RecordDefaults
		if prov := cfg.FindProvider(rec.ProviderID); prov != nil {
			pd = prov.Defaults
		}
		result[i] = MergeDefaults(rec, pd)
	}
	return result
}

