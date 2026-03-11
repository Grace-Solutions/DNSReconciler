package config

import "testing"

func TestValidateAcceptsMinimalConfig(t *testing.T) {
	cfg := Config{
		Settings: SettingsConfig{
			Runtime: RuntimeConfig{LogLevel: "Information"},
			Network: NetworkConfig{AddressSources: []AddressSource{{Priority: 1, Type: "publicIPv4", Enabled: true}}},
		},
		Providers: []ProviderEntry{{
			ID:   "cf-1",
			Type: "cloudflare",
		}},
		Records: []RecordTemplate{{
			RecordID:   "public-api",
			ProviderID: "cf-1",
			Zone:       "example.com",
			Type:       "A",
			Name:       "api.example.com",
			Content:    "${SELECTED_IPV4}",
		}},
	}
	cfg.ApplyBuiltInDefaults()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected config to validate, got error: %v", err)
	}
}

func TestValidateRejectsDuplicatePriorities(t *testing.T) {
	cfg := Config{
		Settings: SettingsConfig{
			Runtime: RuntimeConfig{LogLevel: "Information"},
			Network: NetworkConfig{AddressSources: []AddressSource{
				{Priority: 1, Type: "publicIPv4", Enabled: true},
				{Priority: 1, Type: "rfc1918IPv4", Enabled: true},
			}},
		},
		Providers: []ProviderEntry{{ID: "cf-1", Type: "cloudflare"}},
		Records: []RecordTemplate{{
			RecordID:   "public-api",
			ProviderID: "cf-1",
			Zone:       "example.com",
			Type:       "A",
			Name:       "api.example.com",
			Content:    "${SELECTED_IPV4}",
		}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate priority validation error")
	}
}

func TestValidateSkipsUnknownProvider(t *testing.T) {
	cfg := Config{
		Settings: SettingsConfig{
			Runtime: RuntimeConfig{LogLevel: "Information"},
		},
		Providers: []ProviderEntry{{ID: "cf-1", Type: "cloudflare"}},
		Records: []RecordTemplate{{
			RecordID:   "rec-1",
			ProviderID: "nonexistent",
			Zone:       "example.com",
			Type:       "A",
			Name:       "a.example.com",
			Content:    "${SELECTED_IPV4}",
		}},
	}
	cfg.ApplyBuiltInDefaults()

	// Unknown provider is now non-fatal — validation should pass
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected unknown provider to be skipped, got error: %v", err)
	}
}

func TestValidateSkipsDisabledRecords(t *testing.T) {
	disabled := false
	cfg := Config{
		Settings: SettingsConfig{
			Runtime: RuntimeConfig{LogLevel: "Information"},
		},
		Providers: []ProviderEntry{{ID: "cf-1", Type: "cloudflare"}},
		Records: []RecordTemplate{{
			RecordID:   "rec-disabled",
			ProviderID: "nonexistent",
			Enabled:    &disabled,
			Zone:       "example.com",
			Type:       "A",
			Name:       "a.example.com",
			Content:    "${SELECTED_IPV4}",
		}},
	}
	cfg.ApplyBuiltInDefaults()

	// Disabled records should be gated before provider lookup
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected disabled record to be skipped, got error: %v", err)
	}
}

func TestValidateRejectsDuplicateProviderIDs(t *testing.T) {
	cfg := Config{
		Settings: SettingsConfig{Runtime: RuntimeConfig{LogLevel: "Information"}},
		Providers: []ProviderEntry{
			{ID: "cf-1", Type: "cloudflare"},
			{ID: "cf-1", Type: "cloudflare"},
		},
	}
	cfg.ApplyBuiltInDefaults()

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate provider id validation error")
	}
}