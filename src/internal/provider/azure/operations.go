package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// basePath returns the Azure DNS API base path for the configured zone.
func (p *Provider) basePath() string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/dnsZones/%s",
		p.subscriptionID, p.resourceGroup, p.zoneName)
}

// ListRecords queries Azure DNS for record sets matching the filter.
func (p *Provider) ListRecords(ctx context.Context, filter core.RecordFilter) ([]core.Record, error) {
	recordType := filter.Type
	path := fmt.Sprintf("%s/%s?api-version=%s", p.basePath(), recordType, apiVersion)

	var resp recordSetListResult
	if err := p.doJSON(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("azure list records: %w", err)
	}

	var records []core.Record
	for _, rs := range resp.Value {
		recs := fromAzureRecordSet(rs, filter.Zone, recordType)
		// Client-side name filter
		for _, r := range recs {
			if filter.Name != "" && !strings.EqualFold(r.Name, filter.Name) {
				continue
			}
			records = append(records, r)
		}
	}

	p.logger.Debug(fmt.Sprintf("Azure: listed %d records for zone %s (filter: name=%q type=%q)", len(records), p.zoneName, filter.Name, filter.Type))
	return records, nil
}

// CreateRecord creates a DNS record set in Azure DNS.
func (p *Provider) CreateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	return p.upsertRecord(ctx, record)
}

// UpdateRecord updates a DNS record set in Azure DNS.
func (p *Provider) UpdateRecord(ctx context.Context, record core.Record) (core.Record, error) {
	return p.upsertRecord(ctx, record)
}

// DeleteRecord removes a DNS record set from Azure DNS.
func (p *Provider) DeleteRecord(ctx context.Context, record core.Record) error {
	relName := relativeRecordName(record.Name, p.zoneName)
	path := fmt.Sprintf("%s/%s/%s?api-version=%s", p.basePath(), record.Type, relName, apiVersion)
	if err := p.doJSON(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("azure delete record: %w", err)
	}
	p.logger.Information(fmt.Sprintf("Azure: deleted %s record set %s", record.Type, record.Name))
	return nil
}

// upsertRecord creates or updates a record set via PUT.
func (p *Provider) upsertRecord(ctx context.Context, record core.Record) (core.Record, error) {
	relName := relativeRecordName(record.Name, p.zoneName)
	path := fmt.Sprintf("%s/%s/%s?api-version=%s", p.basePath(), record.Type, relName, apiVersion)

	body := toAzureRecordSet(record)

	var resp recordSet
	if err := p.doJSON(ctx, "PUT", path, body, &resp); err != nil {
		return core.Record{}, fmt.Errorf("azure upsert record: %w", err)
	}

	p.logger.Information(fmt.Sprintf("Azure: upserted %s record set %s → %s", record.Type, record.Name, record.Content))
	result := record
	result.ProviderRecordID = resp.ID
	return result, nil
}

// relativeRecordName extracts the relative name (e.g. "api" from "api.example.com" in zone "example.com").
func relativeRecordName(fqdn, zoneName string) string {
	fqdn = strings.TrimSuffix(fqdn, ".")
	zoneName = strings.TrimSuffix(zoneName, ".")
	if strings.EqualFold(fqdn, zoneName) {
		return "@"
	}
	suffix := "." + zoneName
	if strings.HasSuffix(strings.ToLower(fqdn), strings.ToLower(suffix)) {
		return fqdn[:len(fqdn)-len(suffix)]
	}
	return fqdn
}

// fqdnFromRelative reconstructs the FQDN from a relative name and zone.
func fqdnFromRelative(name, zoneName string) string {
	if name == "@" {
		return zoneName
	}
	return name + "." + zoneName
}

// toAzureRecordSet converts an internal record to an Azure DNS record set body.
func toAzureRecordSet(r core.Record) recordSet {
	props := recordSetProperties{
		TTL: r.TTL,
	}

	// Map tags to Azure metadata
	if len(r.Tags) > 0 {
		props.Metadata = make(map[string]string)
		for _, t := range r.Tags {
			props.Metadata[t.Name] = t.Value
		}
	}

	switch r.Type {
	case "A":
		props.ARecords = []aRecord{{IPv4Address: r.Content}}
	case "AAAA":
		props.AAAARecords = []aaaaRecord{{IPv6Address: r.Content}}
	case "CNAME":
		props.CNAMERecord = &cnameRecord{CNAME: r.Content}
	case "TXT":
		props.TXTRecords = []txtRecord{{Value: []string{r.Content}}}
	}

	return recordSet{Properties: props}
}

// fromAzureRecordSet converts an Azure DNS record set to internal records.
func fromAzureRecordSet(rs recordSet, zone, recordType string) []core.Record {
	fqdn := fqdnFromRelative(rs.Name, zone)

	// Convert metadata back to tags
	var tags []core.Tag
	for k, v := range rs.Properties.Metadata {
		tags = append(tags, core.Tag{Name: k, Value: v})
	}

	base := core.Record{
		Provider:         providerName,
		Zone:             zone,
		Type:             recordType,
		Name:             fqdn,
		TTL:              rs.Properties.TTL,
		Enabled:          true,
		Tags:             tags,
		ProviderRecordID: rs.ID,
	}

	var records []core.Record
	switch recordType {
	case "A":
		for _, a := range rs.Properties.ARecords {
			r := base
			r.Content = a.IPv4Address
			records = append(records, r)
		}
	case "AAAA":
		for _, a := range rs.Properties.AAAARecords {
			r := base
			r.Content = a.IPv6Address
			records = append(records, r)
		}
	case "CNAME":
		if rs.Properties.CNAMERecord != nil {
			r := base
			r.Content = rs.Properties.CNAMERecord.CNAME
			records = append(records, r)
		}
	case "TXT":
		for _, t := range rs.Properties.TXTRecords {
			r := base
			r.Content = strings.Join(t.Value, "")
			records = append(records, r)
		}
	}

	return records
}

