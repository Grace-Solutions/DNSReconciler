package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultConfigJSON is a human-friendly starter config written when no config
// file exists. It includes a sample Cloudflare provider and record so new
// users can see the shape and edit it with real values.
const defaultConfigJSON = `{
    "settings": {
        "runtime": {
            "reconcileIntervalSeconds": 120,
            "statePath":                "./state.json",
            "cleanupOnShutdown":        false,
            "logLevel":                 "Information",
            "dryRun":                   false
        },
        "network": {
            "addressSources": [
                { "priority": 1, "type": "publicIPv4",  "enabled": true },
                { "priority": 2, "type": "rfc1918IPv4", "enabled": true }
            ]
        }
    },

    "providers": [
        {
            "providerId":   "535c8629-764e-456b-9946-0bce7ea1e739",
            "friendlyName": "cloudflare-primary",
            "type":         "cloudflare",
            "enabled":      true,
            "apiToken":     "env:CF_API_TOKEN",
            "zoneId":       "env:CF_ZONE_ID",
            "zone":         "example.com",
            "ttl":          120,
            "proxied":      false,
            "comment":      "Managed by dnsreconciler on ${HOSTNAME}"
        }
    ],

    "records": [
        {
            "providerId": "535c8629-764e-456b-9946-0bce7ea1e739",
            "recordId":   "02275d10-d877-486c-9773-346555c1964a",
            "enabled":    true,
            "type":       "A",
            "name":       "${HOSTNAME}.${ZONE}",
            "content":    "${SELECTED_IPV4}"
        }
    ]
}
`

// WriteDefault writes the starter config template to the given path, creating
// any parent directories as needed. It does NOT overwrite an existing file.
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

	if err := os.WriteFile(path, []byte(defaultConfigJSON), 0o644); err != nil {
		return fmt.Errorf("write default config %q: %w", path, err)
	}
	return nil
}

