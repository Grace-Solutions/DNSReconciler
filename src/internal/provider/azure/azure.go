// Package azure implements the Azure DNS provider (§9).
// Azure DNS record sets support metadata (key-value pairs) which are
// used as structured tags for the ownership model.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
)

const (
	defaultManagementURL = "https://management.azure.com"
	apiVersion           = "2018-05-01"
	providerName         = "azure"
)

// Provider implements core.Provider for Azure DNS.
type Provider struct {
	logger         *logging.Logger
	httpClient     *http.Client
	tokenCache     *tokenCache
	managementURL  string
	subscriptionID string
	resourceGroup  string
	zoneName       string
}

// New creates an Azure DNS provider from the providerDefaults config.
// Required: "tenantId", "clientId", "clientSecret", "subscriptionId", "resourceGroup", "zoneName".
// Optional: "managementUrl" (for testing/sovereign clouds).
func New(cfg map[string]any, logger *logging.Logger) (core.Provider, error) {
	tenantID, err := provider.RequireCredential(cfg, "tenantId")
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}
	clientID, err := provider.RequireCredential(cfg, "clientId")
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}
	clientSecret, err := provider.RequireCredential(cfg, "clientSecret")
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}
	subscriptionID, err := provider.RequireCredential(cfg, "subscriptionId")
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}
	resourceGroup, err := provider.RequireCredential(cfg, "resourceGroup")
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}
	zoneName, err := provider.RequireCredential(cfg, "zoneName")
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}

	mgmtURL := provider.OptionalString(cfg, "managementUrl", defaultManagementURL)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	tc := newTokenCache(httpClient, tenantID, clientID, clientSecret)

	return &Provider{
		logger:         logger,
		httpClient:     httpClient,
		tokenCache:     tc,
		managementURL:  mgmtURL,
		subscriptionID: subscriptionID,
		resourceGroup:  resourceGroup,
		zoneName:       zoneName,
	}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) ValidateConfig(cfg map[string]any) error {
	for _, key := range []string{"tenantId", "clientId", "clientSecret", "subscriptionId", "resourceGroup", "zoneName"} {
		if _, err := provider.RequireCredential(cfg, key); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsComments:                false,
		SupportsStructuredTags:          true, // Azure DNS metadata
		SupportsServerSideCommentFilter: false,
		SupportsServerSideTagFilter:     false,
		SupportsPerRecordUpdates:        false,
		SupportsRRSetUpdates:            true,
		SupportsWildcardRecords:         true,
		SupportsProxiedFlag:             false,
		SupportsBatchChanges:            false,
	}
}

// doJSON executes an authenticated request against the Azure Management API.
func (p *Provider) doJSON(ctx context.Context, method, path string, body any, dest any) error {
	token, err := p.tokenCache.getToken(ctx)
	if err != nil {
		return err
	}

	url := p.managementURL + path
	var bodyReader io.Reader
	var bodySnapshot string
	if body != nil {
		encoded, encErr := json.Marshal(body)
		if encErr != nil {
			return fmt.Errorf("azure: marshal request: %w", encErr)
		}
		bodySnapshot = string(encoded)
		bodyReader = strings.NewReader(bodySnapshot)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("azure: create request %s %s: %w", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if bodySnapshot != "" {
		p.logger.Debug(fmt.Sprintf("HTTP request: %s %s body=%s", method, url, bodySnapshot))
	} else {
		p.logger.Debug(fmt.Sprintf("HTTP request: %s %s", method, url))
	}
	start := time.Now()

	resp, err := p.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		p.logger.Warning(fmt.Sprintf("HTTP request failed: %s %s (%s): %s", method, url, elapsed, err))
		return fmt.Errorf("azure: %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("azure: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.logger.Warning(fmt.Sprintf("HTTP response: %s %s → %d (%s)", method, url, resp.StatusCode, elapsed))
		var errResp azureErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return fmt.Errorf("azure: %s %s: %s — %s", method, url, errResp.Error.Code, errResp.Error.Message)
		}
		snippet := string(respBody)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return fmt.Errorf("azure: %s %s returned %d: %s", method, url, resp.StatusCode, snippet)
	}

	p.logger.Debug(fmt.Sprintf("HTTP response: %s %s → %d (%s)", method, url, resp.StatusCode, elapsed))

	if dest != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("azure: decode response: %w", err)
		}
	}
	return nil
}

