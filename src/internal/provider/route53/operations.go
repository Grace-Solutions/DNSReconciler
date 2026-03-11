package route53

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// canonicalName ensures a trailing dot (Route53 convention).
func canonicalName(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// stripTrailingDot removes a trailing dot for internal consistency.
func stripTrailingDot(name string) string {
	return strings.TrimSuffix(name, ".")
}

// ListRecords queries Route53 for resource record sets matching the filter.
func (p *Provider) ListRecords(ctx context.Context, filter core.RecordFilter) ([]core.Record, error) {
	path := fmt.Sprintf("/2013-04-01/hostedzone/%s/rrset", p.hostedZoneID)
	if filter.Name != "" {
		path += fmt.Sprintf("?name=%s&type=%s&maxitems=100", canonicalName(filter.Name), filter.Type)
	}

	var resp listResourceRecordSetsResponse
	if err := p.doXML(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("route53 list records: %w", err)
	}

	var records []core.Record
	for _, rrset := range resp.ResourceRecordSets {
		name := stripTrailingDot(rrset.Name)
		// Filter to only matching name+type if specified
		if filter.Name != "" && !strings.EqualFold(name, stripTrailingDot(filter.Name)) {
			continue
		}
		if filter.Type != "" && rrset.Type != filter.Type {
			continue
		}

		for _, rr := range rrset.ResourceRecords {
			records = append(records, core.Record{
				Provider:         providerName,
				Zone:             filter.Zone,
				Type:             rrset.Type,
				Name:             name,
				Content:          stripTrailingDot(rr.Value),
				TTL:              rrset.TTL,
				Enabled:          true,
				ProviderRecordID: fmt.Sprintf("%s|%s|%s", filter.Zone, name, rrset.Type),
			})
		}
	}

	return records, nil
}

// CreateRecord creates a DNS record via Route53 UPSERT change batch.
func (p *Provider) CreateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	return p.changeRecord(ctx, "UPSERT", record)
}

// UpdateRecord updates a DNS record via Route53 UPSERT change batch.
func (p *Provider) UpdateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	return p.changeRecord(ctx, "UPSERT", record)
}

// DeleteRecord removes a DNS record via Route53 DELETE change batch.
func (p *Provider) DeleteRecord(ctx context.Context, record core.Record) error {
	_, err := p.changeRecord(ctx, "DELETE", record)
	return err
}

// changeRecord executes a ChangeResourceRecordSets request.
func (p *Provider) changeRecord(ctx context.Context, action string, record core.Record) (core.Record, error) {
	content := record.Content
	// Route53 requires trailing dot for CNAME values
	if record.Type == "CNAME" && !strings.HasSuffix(content, ".") {
		content += "."
	}
	// TXT records must be quoted
	if record.Type == "TXT" && !strings.HasPrefix(content, "\"") {
		content = fmt.Sprintf("%q", content)
	}

	reqBody := changeResourceRecordSetsRequest{
		XMLNS: "https://route53.amazonaws.com/doc/2013-04-01/",
		ChangeBatch: changeBatch{
			Comment: "Managed by dnsreconciler",
			Changes: []change{{
				Action: action,
				ResourceRecordSet: resourceRecordSet{
					Name: canonicalName(record.Name),
					Type: record.Type,
					TTL:  record.TTL,
					ResourceRecords: []resourceRecord{
						{Value: content},
					},
				},
			}},
		},
	}

	body, err := xml.MarshalIndent(reqBody, "", "  ")
	if err != nil {
		return core.Record{}, fmt.Errorf("route53 marshal change: %w", err)
	}

	path := fmt.Sprintf("/2013-04-01/hostedzone/%s/rrset", p.hostedZoneID)
	var resp changeResourceRecordSetsResponse
	if err := p.doXML(ctx, "POST", path, body, &resp); err != nil {
		return core.Record{}, fmt.Errorf("route53 %s record: %w", strings.ToLower(action), err)
	}

	p.logger.Debug(fmt.Sprintf("Route53: %s record %s %s (change: %s, status: %s)",
		strings.ToLower(action), record.Type, record.Name, resp.ChangeInfo.ID, resp.ChangeInfo.Status))

	result := record
	result.ProviderRecordID = fmt.Sprintf("%s|%s|%s", record.Zone, record.Name, record.Type)
	return result, nil
}

