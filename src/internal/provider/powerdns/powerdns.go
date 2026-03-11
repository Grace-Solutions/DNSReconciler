// Package powerdns implements the PowerDNS Authoritative Server provider (§5.1, §9).
// PowerDNS uses RRset-oriented operations. Ownership is managed at the record
// content/comment level since PowerDNS does not support tags (§11.3).
package powerdns

import (
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
)

const (
	defaultBaseURL  = "http://localhost:8081"
	defaultServerID = "localhost"
	providerName    = "powerdns"
)

// Provider implements core.Provider for PowerDNS Authoritative Server.
type Provider struct {
	logger   *logging.Logger
	client   *provider.APIClient
	serverID string
}

// New creates a PowerDNS provider from the providerDefaults config.
// Required config keys: "apiKey".
// Optional: "baseUrl" (default http://localhost:8081), "serverId" (default "localhost").
func New(cfg map[string]any, logger *logging.Logger) (core.Provider, error) {
	apiKey, err := provider.RequireCredential(cfg, "apiKey")
	if err != nil {
		return nil, fmt.Errorf("powerdns: %w", err)
	}

	baseURL := provider.OptionalString(cfg, "baseUrl", defaultBaseURL)
	serverID := provider.OptionalString(cfg, "serverId", defaultServerID)

	client := provider.NewAPIClient(baseURL, map[string]string{
		"X-API-Key": apiKey,
	})

	return &Provider{logger: logger, client: client, serverID: serverID}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) ValidateConfig(cfg map[string]any) error {
	_, err := provider.RequireCredential(cfg, "apiKey")
	return err
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsComments:                true,
		SupportsStructuredTags:          false,
		SupportsServerSideCommentFilter: false,
		SupportsServerSideTagFilter:     false,
		SupportsPerRecordUpdates:        false,
		SupportsRRSetUpdates:            true,
		SupportsWildcardRecords:         true,
		SupportsProxiedFlag:             false,
		SupportsBatchChanges:            true,
	}
}

