package config

import "testing"

func TestValidateAcceptsMinimalSpecAlignedConfig(t *testing.T) {
	config := Config{
		Version: 1,
		Runtime: RuntimeConfig{LogLevel: "Information"},
		Network: NetworkConfig{AddressSources: []AddressSource{{Priority: 1, Type: "publicIPv4", Enabled: true}}},
		Records: []RecordTemplate{{
			ID:       "public-api",
			Provider: "cloudflare",
			Zone:     "example.com",
			Type:     "A",
			Name:     "api.example.com",
			Content:  "${SELECTED_IPV4}",
		}},
	}
	config.ApplyBuiltInDefaults()

	if err := config.Validate(); err != nil {
		t.Fatalf("expected config to validate, got error: %v", err)
	}
}

func TestValidateRejectsDuplicatePriorities(t *testing.T) {
	config := Config{
		Version: 1,
		Runtime: RuntimeConfig{LogLevel: "Information"},
		Network: NetworkConfig{AddressSources: []AddressSource{
			{Priority: 1, Type: "publicIPv4", Enabled: true},
			{Priority: 1, Type: "rfc1918IPv4", Enabled: true},
		}},
		Records: []RecordTemplate{{
			ID:       "public-api",
			Provider: "cloudflare",
			Zone:     "example.com",
			Type:     "A",
			Name:     "api.example.com",
			Content:  "${SELECTED_IPV4}",
		}},
	}

	if err := config.Validate(); err == nil {
		t.Fatal("expected duplicate priority validation error")
	}
}

func TestValidateRequiresExplicitVersion(t *testing.T) {
	config := Config{}
	config.ApplyBuiltInDefaults()

	if err := config.Validate(); err == nil {
		t.Fatal("expected unsupported version error")
	}
}