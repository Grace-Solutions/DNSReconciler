// Package route53 implements the AWS Route53 provider (§9).
// Route53 uses XML APIs with AWS Signature V4 authentication.
// Ownership is state-file based since Route53 does not support
// tags or comments on individual resource records.
package route53

import (
	"bytes"
	"context"
	"encoding/xml"
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
	defaultBaseURL = "https://route53.amazonaws.com"
	defaultRegion  = "us-east-1"
	providerName   = "route53"
)

// Provider implements core.Provider for AWS Route53.
type Provider struct {
	logger       *logging.Logger
	httpClient   *http.Client
	signer       *awsSigner
	baseURL      string
	hostedZoneID string
}

// New creates a Route53 provider from the providerDefaults config.
// Required config keys: "accessKeyId", "secretAccessKey", "hostedZoneId".
// Optional: "region" (default us-east-1), "baseUrl" (for testing).
func New(cfg map[string]any, logger *logging.Logger) (core.Provider, error) {
	accessKey, err := provider.RequireCredential(cfg, "accessKeyId")
	if err != nil {
		return nil, fmt.Errorf("route53: %w", err)
	}
	secretKey, err := provider.RequireCredential(cfg, "secretAccessKey")
	if err != nil {
		return nil, fmt.Errorf("route53: %w", err)
	}
	hostedZoneID, err := provider.RequireCredential(cfg, "hostedZoneId")
	if err != nil {
		return nil, fmt.Errorf("route53: %w", err)
	}

	// Strip /hostedzone/ prefix if present
	hostedZoneID = strings.TrimPrefix(hostedZoneID, "/hostedzone/")

	region := provider.OptionalString(cfg, "region", defaultRegion)
	baseURL := provider.OptionalString(cfg, "baseUrl", defaultBaseURL)

	return &Provider{
		logger:       logger,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		signer:       &awsSigner{accessKeyID: accessKey, secretAccessKey: secretKey, region: region},
		baseURL:      baseURL,
		hostedZoneID: hostedZoneID,
	}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) ValidateConfig(cfg map[string]any) error {
	for _, key := range []string{"accessKeyId", "secretAccessKey", "hostedZoneId"} {
		if _, err := provider.RequireCredential(cfg, key); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsComments:                false,
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

// doXML executes a signed request and decodes the XML response.
func (p *Provider) doXML(ctx context.Context, method, path string, body []byte, dest any) error {
	url := p.baseURL + path
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("route53: create request %s %s: %w", method, url, err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/xml")
	}

	p.signer.sign(req, body)

	p.logger.Debug(fmt.Sprintf("HTTP request: %s %s", method, path))
	start := time.Now()

	resp, err := p.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		p.logger.Warning(fmt.Sprintf("HTTP request failed: %s %s (%s): %s", method, path, elapsed, err))
		return fmt.Errorf("route53: %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("route53: read response %s %s: %w", method, url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.logger.Warning(fmt.Sprintf("HTTP response: %s %s → %d (%s)", method, path, resp.StatusCode, elapsed))
		var errResp errorResponse
		if xml.Unmarshal(respBody, &errResp) == nil {
			return fmt.Errorf("route53: %s %s: %s — %s", method, path, errResp.Error.Code, errResp.Error.Message)
		}
		snippet := string(respBody)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return fmt.Errorf("route53: %s %s returned %d: %s", method, path, resp.StatusCode, snippet)
	}

	p.logger.Debug(fmt.Sprintf("HTTP response: %s %s → %d (%s)", method, path, resp.StatusCode, elapsed))

	if dest != nil && len(respBody) > 0 {
		if err := xml.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("route53: decode response %s %s: %w", method, path, err)
		}
	}
	return nil
}

