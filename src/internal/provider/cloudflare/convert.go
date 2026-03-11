package cloudflare

import (
	"fmt"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
)

// toCFRecord converts an internal record to a Cloudflare API record body.
func toCFRecord(r core.Record) cfDNSRecord {
	tags := make([]string, len(r.Tags))
	for i, t := range r.Tags {
		tags[i] = fmt.Sprintf("%s:%s", t.Name, t.Value)
	}
	return cfDNSRecord{
		Type:    r.Type,
		Name:    r.Name,
		Content: r.Content,
		TTL:     r.TTL,
		Proxied: r.Proxied,
		Comment: r.Comment,
		Tags:    tags,
	}
}

// fromCFRecord converts a Cloudflare API record to an internal record.
func fromCFRecord(cf cfDNSRecord) core.Record {
	tags := make([]core.Tag, len(cf.Tags))
	for i, raw := range cf.Tags {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 {
			tags[i] = core.Tag{Name: parts[0], Value: parts[1]}
		} else {
			tags[i] = core.Tag{Name: raw, Value: ""}
		}
	}
	return core.Record{
		Provider:         providerName,
		ProviderRecordID: cf.ID,
		Type:             cf.Type,
		Name:             cf.Name,
		Content:          cf.Content,
		TTL:              cf.TTL,
		Proxied:          cf.Proxied,
		Comment:          cf.Comment,
		Tags:             tags,
	}
}

