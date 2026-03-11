package cloudflare

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// ListRecords queries Cloudflare for DNS records matching the filter.
func (p *Provider) ListRecords(ctx context.Context, filter core.RecordFilter) ([]core.Record, error) {
	params := url.Values{}
	if filter.Name != "" {
		params.Set("name", filter.Name)
	}
	if filter.Type != "" {
		params.Set("type", filter.Type)
	}
	params.Set("per_page", "100")

	// Use tag-based filtering if ownership tags are provided (§11.2, §11.3)
	for _, tag := range filter.Tags {
		params.Add("tag-match", fmt.Sprintf("%s:%s", tag.Name, tag.Value))
	}

	path := fmt.Sprintf("/zones/%s/dns_records?%s", p.zoneID, params.Encode())
	var resp cfListResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare list records: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("cloudflare list records: API returned errors: %v", resp.Errors)
	}

	records := make([]core.Record, len(resp.Result))
	for i, r := range resp.Result {
		records[i] = fromCFRecord(r)
	}
	p.logger.Debug(fmt.Sprintf("Cloudflare: listed %d records for zone %s (filter: name=%q type=%q)", len(records), p.zoneID, filter.Name, filter.Type))
	return records, nil
}

// CreateRecord creates a DNS record in Cloudflare.
func (p *Provider) CreateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	body := toCFRecord(record)
	path := fmt.Sprintf("/zones/%s/dns_records", p.zoneID)

	var resp cfSingleResponse
	if _, err := p.client.Do(ctx, "POST", path, body, &resp); err != nil {
		return core.Record{}, fmt.Errorf("cloudflare create record: %w", err)
	}
	if !resp.Success {
		return core.Record{}, fmt.Errorf("cloudflare create record: API returned errors: %v", resp.Errors)
	}

	p.logger.Information(fmt.Sprintf("Cloudflare: created %s record %s → %s (id: %s)", record.Type, record.Name, record.Content, resp.Result.ID))
	return fromCFRecord(resp.Result), nil
}

// UpdateRecord updates an existing DNS record in Cloudflare.
func (p *Provider) UpdateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	if record.ProviderRecordID == "" {
		return core.Record{}, fmt.Errorf("cloudflare update: missing provider record ID")
	}
	body := toCFRecord(record)
	path := fmt.Sprintf("/zones/%s/dns_records/%s", p.zoneID, record.ProviderRecordID)

	var resp cfSingleResponse
	if _, err := p.client.Do(ctx, "PUT", path, body, &resp); err != nil {
		return core.Record{}, fmt.Errorf("cloudflare update record: %w", err)
	}
	if !resp.Success {
		return core.Record{}, fmt.Errorf("cloudflare update record: API returned errors: %v", resp.Errors)
	}

	p.logger.Information(fmt.Sprintf("Cloudflare: updated %s record %s → %s (id: %s)", record.Type, record.Name, record.Content, resp.Result.ID))
	return fromCFRecord(resp.Result), nil
}

// DeleteRecord removes a DNS record from Cloudflare.
func (p *Provider) DeleteRecord(ctx context.Context, record core.Record) error {
	if record.ProviderRecordID == "" {
		return fmt.Errorf("cloudflare delete: missing provider record ID")
	}
	path := fmt.Sprintf("/zones/%s/dns_records/%s", p.zoneID, record.ProviderRecordID)

	if _, err := p.client.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("cloudflare delete record: %w", err)
	}
	p.logger.Information(fmt.Sprintf("Cloudflare: deleted %s record %s (id: %s)", record.Type, record.Name, record.ProviderRecordID))
	return nil
}

