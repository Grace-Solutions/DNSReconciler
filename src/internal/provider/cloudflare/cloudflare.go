// Package cloudflare implements the Cloudflare DNS provider (§5.1, §9).
package cloudflare

import (
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
)

const (
	defaultBaseURL = "https://api.cloudflare.com/client/v4"
	providerName   = "cloudflare"
)

// Provider implements core.Provider for Cloudflare DNS API v4.
type Provider struct {
	logger *logging.Logger
	client *provider.APIClient
	zoneID string
}

// New creates a Cloudflare provider from the providerDefaults config.
// Required config keys: "apiToken", "zoneId".
// Optional: "baseUrl" (for testing).
func New(cfg map[string]any, logger *logging.Logger) (core.Provider, error) {
	token, err := provider.RequireCredential(cfg, "apiToken")
	if err != nil {
		return nil, fmt.Errorf("cloudflare: %w", err)
	}
	zoneID, err := provider.RequireCredential(cfg, "zoneId")
	if err != nil {
		return nil, fmt.Errorf("cloudflare: %w", err)
	}

	baseURL := provider.OptionalString(cfg, "baseUrl", defaultBaseURL)

	client := provider.NewAPIClient(baseURL, map[string]string{
		"Authorization": "Bearer " + token,
	})

	return &Provider{logger: logger, client: client, zoneID: zoneID}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) ValidateConfig(cfg map[string]any) error {
	if _, err := provider.RequireCredential(cfg, "apiToken"); err != nil {
		return err
	}
	if _, err := provider.RequireCredential(cfg, "zoneId"); err != nil {
		return err
	}
	return nil
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsComments:                true,
		SupportsStructuredTags:          true,
		SupportsServerSideCommentFilter: false,
		SupportsServerSideTagFilter:     true,
		SupportsPerRecordUpdates:        true,
		SupportsRRSetUpdates:            false,
		SupportsWildcardRecords:         true,
		SupportsProxiedFlag:             true,
		SupportsBatchChanges:            false,
	}
}

