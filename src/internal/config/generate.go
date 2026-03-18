package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultConfigJSON is a safe starter config written when no config file
// exists. All providers and records are disabled so the application starts
// without errors and idles until the user configures it with real values.
const defaultConfigJSON = `{
    "settings": {
        "runtime": {
            "schedule":          "0 0 */4 * * *",
            "jitter":            "auto",
            "timezone":          "UTC",
            "statePath":         "/state/state.json",
            "cleanupOnShutdown": false,
            "logLevel":          "Information",
            "dryRun":            false
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
            "providerId":   "00000000-0000-0000-0000-000000000000",
            "friendlyName": "example-cloudflare",
            "type":         "cloudflare",
            "enabled":      false,
            "apiToken":     "env:CF_API_TOKEN",
            "zoneId":       "env:CF_ZONE_ID",
            "zone":         "example.com",
            "ttl":          120,
            "proxied":      false,
            "comment":      "{'hostname': '${HOSTNAME}', 'nodeId': '${NODE_ID}'}"
        }
    ],

    "records": [
        {
            "providerId": "00000000-0000-0000-0000-000000000000",
            "recordId":   "00000000-0000-0000-0000-000000000001",
            "enabled":    false,
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

