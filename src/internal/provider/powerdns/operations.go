package powerdns

import (
	"context"
	"fmt"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// canonicalName ensures the name ends with a dot (PowerDNS convention).
func canonicalName(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// ListRecords queries PowerDNS for records in the zone matching the filter.
func (p *Provider) ListRecords(ctx context.Context, filter core.RecordFilter) ([]core.Record, error) {
	zone := canonicalName(filter.Zone)
	path := fmt.Sprintf("/api/v1/servers/%s/zones/%s", p.serverID, zone)

	var resp pdnsZoneResponse
	if _, err := p.client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("powerdns list records: %w", err)
	}

	var records []core.Record
	for _, rrset := range resp.RRsets {
		if filter.Type != "" && rrset.Type != filter.Type {
			continue
		}
		if filter.Name != "" && rrset.Name != canonicalName(filter.Name) {
			continue
		}
		for _, rec := range rrset.Records {
			comment := ""
			if len(rrset.Comments) > 0 {
				comment = rrset.Comments[0].Content
			}
			records = append(records, core.Record{
				Provider:         providerName,
				Zone:             filter.Zone,
				Type:             rrset.Type,
				Name:             strings.TrimSuffix(rrset.Name, "."),
				Content:          rec.Content,
				TTL:              rrset.TTL,
				Enabled:          !rec.Disabled,
				Comment:          comment,
				ProviderRecordID: fmt.Sprintf("%s|%s|%s", rrset.Name, rrset.Type, rec.Content),
			})
		}
	}
	return records, nil
}

// CreateRecord creates a DNS record via PowerDNS PATCH (RRset REPLACE).
func (p *Provider) CreateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	return p.upsertRecord(ctx, record, "REPLACE")
}

// UpdateRecord updates a DNS record via PowerDNS PATCH (RRset REPLACE).
func (p *Provider) UpdateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	return p.upsertRecord(ctx, record, "REPLACE")
}

// DeleteRecord removes a DNS record via PowerDNS PATCH (RRset DELETE).
func (p *Provider) DeleteRecord(ctx context.Context, record core.Record) error {
	zone := canonicalName(record.Zone)
	body := pdnsPatchBody{
		RRsets: []pdnsRRset{{
			Name:       canonicalName(record.Name),
			Type:       record.Type,
			Changetype: "DELETE",
		}},
	}
	path := fmt.Sprintf("/api/v1/servers/%s/zones/%s", p.serverID, zone)
	if _, err := p.client.Do(ctx, "PATCH", path, body, nil); err != nil {
		return fmt.Errorf("powerdns delete record: %w", err)
	}
	p.logger.Debug(fmt.Sprintf("PowerDNS: deleted RRset %s %s", record.Type, record.Name))
	return nil
}

// upsertRecord performs a PATCH with changetype REPLACE.
func (p *Provider) upsertRecord(ctx context.Context, record core.Record, changetype string) (core.Record, error) {
	zone := canonicalName(record.Zone)
	name := canonicalName(record.Name)

	var comments []pdnsComment
	if record.Comment != "" {
		comments = []pdnsComment{{Content: record.Comment, Account: "dnsreconciler"}}
	}

	body := pdnsPatchBody{
		RRsets: []pdnsRRset{{
			Name:       name,
			Type:       record.Type,
			TTL:        record.TTL,
			Changetype: changetype,
			Records:    []pdnsRecord{{Content: record.Content, Disabled: !record.Enabled}},
			Comments:   comments,
		}},
	}

	path := fmt.Sprintf("/api/v1/servers/%s/zones/%s", p.serverID, zone)
	if _, err := p.client.Do(ctx, "PATCH", path, body, nil); err != nil {
		return core.Record{}, fmt.Errorf("powerdns upsert record: %w", err)
	}

	result := record
	result.ProviderRecordID = fmt.Sprintf("%s|%s|%s", name, record.Type, record.Content)
	p.logger.Debug(fmt.Sprintf("PowerDNS: %s RRset %s %s", changetype, record.Type, record.Name))
	return result, nil
}

