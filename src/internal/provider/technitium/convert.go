package technitium

import (
	"fmt"
	"net/url"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// fromTechRecord converts a Technitium API record to an internal record.
func fromTechRecord(r techRecord, zone string) core.Record {
	content := ""
	switch r.Type {
	case "A":
		content = r.RData.IPAddress
	case "AAAA":
		content = r.RData.IPv6Address
	case "CNAME":
		content = r.RData.CName
	case "TXT":
		content = r.RData.Text
	}

	return core.Record{
		Provider:         providerName,
		Zone:             zone,
		Type:             r.Type,
		Name:             r.Name,
		Content:          content,
		TTL:              r.TTL,
		Enabled:          !r.Disabled,
		Comment:          r.Comments,
		ProviderRecordID: fmt.Sprintf("%s|%s|%s", zone, r.Name, r.Type),
	}
}

// buildRecordParams constructs the common URL parameters for Technitium record operations.
func buildRecordParams(token string, record core.Record) url.Values {
	params := url.Values{}
	params.Set("token", token)
	params.Set("domain", record.Name)
	params.Set("zone", record.Zone)
	params.Set("type", record.Type)
	params.Set("ttl", fmt.Sprintf("%d", record.TTL))

	cv := contentForType(record)
	if cv != "" {
		switch record.Type {
		case "A":
			params.Set("ipAddress", cv)
		case "AAAA":
			params.Set("ipAddress", cv)
		case "CNAME":
			params.Set("cname", cv)
		case "TXT":
			params.Set("text", cv)
		default:
			params.Set("value", cv)
		}
	}

	if record.Comment != "" {
		params.Set("comments", record.Comment)
	}
	return params
}

// contentForType returns the content value for parameter assignment.
func contentForType(record core.Record) string {
	return record.Content
}

