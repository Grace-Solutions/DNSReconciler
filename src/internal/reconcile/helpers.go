package reconcile

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
)

// fingerprint produces a deterministic hash representing the desired state of a record.
func fingerprint(r core.Record) string {
	parts := []string{
		r.Provider, r.Zone, r.Type, r.Name, r.Content,
		fmt.Sprintf("ttl=%d", r.TTL),
		fmt.Sprintf("proxied=%v", r.Proxied),
		r.Comment, r.OwnershipMode,
	}
	for _, t := range r.Tags {
		parts = append(parts, fmt.Sprintf("tag:%s=%s", t.Name, t.Value))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", hash[:16])
}

// buildOwnershipFilter creates a RecordFilter for the provider query.
func buildOwnershipFilter(desired core.Record, nodeID string) core.RecordFilter {
	return core.RecordFilter{
		Zone: desired.Zone,
		Name: desired.Name,
		Type: desired.Type,
		Ownership: map[string]string{
			"managed-by":         "dnsreconciler",
			"node-id":            nodeID,
			"record-template-id": desired.RecordTemplateID,
		},
	}
}

// findOwnedRecord locates the record we own among the existing set.
// For now, matches by name+type+template ID tag.
func findOwnedRecord(existing []core.Record, desired core.Record) *core.Record {
	for i, rec := range existing {
		if rec.Name == desired.Name && rec.Type == desired.Type {
			if hasMatchingTemplateTag(rec.Tags, desired.RecordTemplateID) {
				return &existing[i]
			}
		}
	}
	return nil
}

// hasMatchingTemplateTag checks if the record has a record-template-id tag matching the desired ID.
func hasMatchingTemplateTag(tags []core.Tag, templateID string) bool {
	for _, t := range tags {
		if t.Name == "record-template-id" && t.Value == templateID {
			return true
		}
	}
	return false
}

// updateState persists the reconciliation result in local state (§21.2 step 11).
func updateState(st *state.File, recordID string, record core.Record, fingerprint, selectedAddr string) {
	if st.Records == nil {
		st.Records = map[string]state.RecordState{}
	}
	st.Records[recordID] = state.RecordState{
		ProviderRecordID:  record.ProviderRecordID,
		DesiredFingerprint: fingerprint,
		SelectedAddress:   selectedAddr,
		LastReconciledUTC: time.Now().UTC().Format(time.RFC3339),
	}
}

// selectByFamily returns addr if it matches the requested family, empty otherwise.
func selectByFamily(addr, family string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return ""
	}
	switch family {
	case "ipv4":
		if ip.To4() != nil {
			return addr
		}
		return ""
	case "ipv6":
		if ip.To4() == nil {
			return addr
		}
		return ""
	default:
		return addr
	}
}

