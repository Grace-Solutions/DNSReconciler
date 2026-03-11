package provider

import (
	"fmt"
	"os"
	"strings"
)

// ResolveCredential resolves a credential value from the provider config map.
// It supports three patterns per §27.2:
//   - direct value:  { "apiToken": "sk-..." }
//   - env variable:  { "apiToken": "env:CF_API_TOKEN" }
//   - file path:     { "apiToken": "file:/run/secrets/cf_token" }
//
// Returns empty string if the key is absent.
func ResolveCredential(cfg map[string]any, key string) (string, error) {
	raw, ok := cfg[key]
	if !ok {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("credential %q must be a string", key)
	}
	value = strings.TrimSpace(value)

	if strings.HasPrefix(value, "env:") {
		envName := strings.TrimPrefix(value, "env:")
		envValue := os.Getenv(envName)
		if envValue == "" {
			return "", fmt.Errorf("environment variable %q (for credential %q) is not set", envName, key)
		}
		return envValue, nil
	}

	if strings.HasPrefix(value, "file:") {
		filePath := strings.TrimPrefix(value, "file:")
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read credential file %q (for %q): %w", filePath, key, err)
		}
		return strings.TrimSpace(string(content)), nil
	}

	return value, nil
}

// RequireCredential is like ResolveCredential but returns an error when the value is empty.
func RequireCredential(cfg map[string]any, key string) (string, error) {
	value, err := ResolveCredential(cfg, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("required credential %q is missing or empty", key)
	}
	return value, nil
}

// OptionalString reads an optional string from the config map.
func OptionalString(cfg map[string]any, key, defaultValue string) string {
	raw, ok := cfg[key]
	if !ok {
		return defaultValue
	}
	s, ok := raw.(string)
	if !ok {
		return defaultValue
	}
	if s == "" {
		return defaultValue
	}
	return s
}

