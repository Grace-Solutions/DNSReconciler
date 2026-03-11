// Package technitium implements the Technitium DNS Server provider (§5.1, §9).
// Technitium API uses query-parameter based endpoints with a session token.
// Ownership is handled via deterministic name+type matching since Technitium
// has limited metadata support (§11.3).
package technitium

import (
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
)

const (
	defaultBaseURL = "http://localhost:5380"
	providerName   = "technitium"
)

// Provider implements core.Provider for Technitium DNS Server.
type Provider struct {
	logger   *logging.Logger
	client   *provider.APIClient
	apiToken string
}

// New creates a Technitium provider from the providerDefaults config.
// Required config keys: "apiToken".
// Optional: "baseUrl" (defaults to http://localhost:5380).
func New(cfg map[string]any, logger *logging.Logger) (core.Provider, error) {
	token, err := provider.RequireCredential(cfg, "apiToken")
	if err != nil {
		return nil, fmt.Errorf("technitium: %w", err)
	}

	baseURL := provider.OptionalString(cfg, "baseUrl", defaultBaseURL)

	client := provider.NewAPIClient(baseURL, map[string]string{})

	return &Provider{logger: logger, client: client, apiToken: token}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) ValidateConfig(cfg map[string]any) error {
	_, err := provider.RequireCredential(cfg, "apiToken")
	return err
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsComments:                true,
		SupportsStructuredTags:          false,
		SupportsServerSideCommentFilter: false,
		SupportsServerSideTagFilter:     false,
		SupportsPerRecordUpdates:        true,
		SupportsRRSetUpdates:            false,
		SupportsWildcardRecords:         true,
		SupportsProxiedFlag:             false,
		SupportsBatchChanges:            false,
	}
}

