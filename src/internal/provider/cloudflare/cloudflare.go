// Package cloudflare implements the Cloudflare DNS provider (§5.1, §9).
package cloudflare

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/provider"
)

const (
	defaultBaseURL = "https://api.cloudflare.com/client/v4"
	providerName   = "cloudflare"

	// planCacheTTL controls how long the zone plan detection result is cached.
	// Plan changes are rare so 24 hours is a reasonable refresh interval.
	planCacheTTL = 24 * time.Hour
)

// Provider implements core.Provider and core.CapabilityRefresher for Cloudflare DNS API v4.
type Provider struct {
	logger   *logging.Logger
	client   *provider.APIClient
	zoneID   string
	zoneName string

	mu            sync.RWMutex
	freePlan      bool      // true when the zone is on Cloudflare's free plan
	planCheckedAt time.Time // when the plan was last checked
}

// New creates a Cloudflare provider from the providerDefaults config.
// Required config keys: "apiToken", "zoneId".
// Optional: "baseUrl" (for testing).
// During initialization the zone's plan is detected via GET /zones/{id}.
// If the zone is on the free plan, structured tag support is disabled and
// the provider falls back to comment-based ownership.
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
	}, logger)

	p := &Provider{logger: logger, client: client, zoneID: zoneID}

	// Detect zone plan to determine tag support.
	logger.Debug(fmt.Sprintf("Cloudflare: detecting zone plan for zone %s", zoneID))
	if err := p.detectZonePlan(context.Background()); err != nil {
		return nil, fmt.Errorf("cloudflare: zone plan detection: %w", err)
	}

	return p, nil
}

// detectZonePlan calls GET /zones/{id} and inspects the plan.
// Free-plan zones do not support DNS record tags; the provider
// automatically falls back to comment-based ownership.
// Caller must NOT hold p.mu.
func (p *Provider) detectZonePlan(ctx context.Context) error {
	path := fmt.Sprintf("/zones/%s", p.zoneID)
	var resp cfZoneResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return fmt.Errorf("get zone: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("get zone: API returned errors: %v", resp.Errors)
	}

	p.zoneName = resp.Result.Name
	planName := strings.ToLower(resp.Result.Plan.Name)
	isFree := strings.Contains(planName, "free")

	p.mu.Lock()
	p.freePlan = isFree
	p.planCheckedAt = time.Now()
	p.mu.Unlock()

	if isFree {
		p.logger.Information(fmt.Sprintf("Cloudflare [Zone: %s] [Plan: free] tags disabled, using comment-based ownership", p.zoneName))
	} else {
		p.logger.Information(fmt.Sprintf("Cloudflare [Zone: %s] [Plan: %s] tag-based ownership enabled", p.zoneName, resp.Result.Plan.Name))
	}
	return nil
}

// RefreshCapabilitiesIfStale re-checks the zone plan if the cached result
// has exceeded planCacheTTL. Implements core.CapabilityRefresher.
func (p *Provider) RefreshCapabilitiesIfStale(ctx context.Context) error {
	p.mu.RLock()
	stale := time.Since(p.planCheckedAt) > planCacheTTL
	p.mu.RUnlock()

	if !stale {
		return nil
	}

	p.logger.Debug("Cloudflare plan cache expired, refreshing zone plan")
	return p.detectZonePlan(ctx)
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
	p.mu.RLock()
	free := p.freePlan
	p.mu.RUnlock()

	return core.ProviderCapabilities{
		SupportsComments:                true,
		SupportsStructuredTags:          !free,
		SupportsServerSideCommentFilter: false,
		SupportsServerSideTagFilter:     !free,
		SupportsPerRecordUpdates:        true,
		SupportsRRSetUpdates:            false,
		SupportsWildcardRecords:         true,
		SupportsProxiedFlag:             true,
		SupportsBatchChanges:            false,
	}
}

