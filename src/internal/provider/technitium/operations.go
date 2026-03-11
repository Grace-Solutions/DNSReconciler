package technitium

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// ListRecords queries Technitium for records in the given zone matching the filter.
func (p *Provider) ListRecords(ctx context.Context, filter core.RecordFilter) ([]core.Record, error) {
	params := url.Values{}
	params.Set("token", p.apiToken)
	params.Set("domain", filter.Name)
	params.Set("zone", filter.Zone)
	if filter.Type != "" {
		params.Set("type", filter.Type)
	}

	path := "/api/zones/records/get?" + params.Encode()
	var resp techListResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("technitium list records: %w", err)
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("technitium list records: %s", resp.ErrorMsg)
	}

	var records []core.Record
	for _, r := range resp.Response.Records {
		records = append(records, fromTechRecord(r, filter.Zone))
	}
	return records, nil
}

// CreateRecord adds a DNS record via the Technitium API.
func (p *Provider) CreateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	params := buildRecordParams(p.apiToken, record)
	path := "/api/zones/records/add?" + params.Encode()

	var resp techResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return core.Record{}, fmt.Errorf("technitium create record: %w", err)
	}
	if resp.Status != "ok" {
		return core.Record{}, fmt.Errorf("technitium create record: %s", resp.ErrorMsg)
	}

	p.logger.Debug(fmt.Sprintf("Technitium: created record %s %s", record.Type, record.Name))
	created := record
	created.ProviderRecordID = fmt.Sprintf("%s|%s|%s", record.Zone, record.Name, record.Type)
	return created, nil
}

// UpdateRecord modifies an existing DNS record in Technitium.
// Technitium update requires both old and new values.
func (p *Provider) UpdateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	params := buildRecordParams(p.apiToken, record)
	// For updates, Technitium needs the domain and type to identify the record.
	// We use the update endpoint with newIpAddress/newDomain parameters.
	params.Set("newIpAddress", contentForType(record))
	params.Set("newDomain", record.Name)
	path := "/api/zones/records/update?" + params.Encode()

	var resp techResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return core.Record{}, fmt.Errorf("technitium update record: %w", err)
	}
	if resp.Status != "ok" {
		return core.Record{}, fmt.Errorf("technitium update record: %s", resp.ErrorMsg)
	}

	p.logger.Debug(fmt.Sprintf("Technitium: updated record %s %s", record.Type, record.Name))
	return record, nil
}

// DeleteRecord removes a DNS record from Technitium.
func (p *Provider) DeleteRecord(ctx context.Context, record core.Record) error {
	params := buildRecordParams(p.apiToken, record)
	path := "/api/zones/records/delete?" + params.Encode()

	var resp techResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return fmt.Errorf("technitium delete record: %w", err)
	}
	if resp.Status != "ok" {
		return fmt.Errorf("technitium delete record: %s", resp.ErrorMsg)
	}

	p.logger.Debug(fmt.Sprintf("Technitium: deleted record %s %s", record.Type, record.Name))
	return nil
}

