package reconcile

import (
	"crypto/sha256"
	"fmt"
	"net"
	"regexp"
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
// It matches by name+type and then checks ownership via tags or comment
// using case-insensitive matching.
//
// Strategy (in priority order):
//  1. If the existing record has tags, check whether every ownership
//     key-value pair appears in the tags (case-insensitive).
//  2. If tag matching fails (or no tags present), fall back to the
//     comment field and check whether every ownership value appears
//     in the comment text (case-insensitive regex).
//  3. If neither tag nor comment matched, fall back to name+type match.
//     This ensures that a record already present on the provider (even
//     without ownership metadata) is treated as a reconciliation
//     candidate rather than triggering a duplicate-create error.
func findOwnedRecord(existing []core.Record, desired core.Record, ownership map[string]string) *core.Record {
	var nameTypeCandidate *core.Record
	for i, rec := range existing {
		if !strings.EqualFold(rec.Name, desired.Name) || !strings.EqualFold(rec.Type, desired.Type) {
			continue
		}
		if matchOwnershipByTags(rec.Tags, ownership) {
			return &existing[i]
		}
		if matchOwnershipByComment(rec.Comment, ownership) {
			return &existing[i]
		}
		// Track first name+type match as fallback candidate.
		if nameTypeCandidate == nil {
			nameTypeCandidate = &existing[i]
		}
	}
	return nameTypeCandidate
}

// matchOwnershipByTags returns true when every key-value pair in the
// ownership map has a corresponding tag on the record (case-insensitive).
func matchOwnershipByTags(tags []core.Tag, ownership map[string]string) bool {
	if len(tags) == 0 || len(ownership) == 0 {
		return false
	}
	for key, val := range ownership {
		if !hasTagCaseInsensitive(tags, key, val) {
			return false
		}
	}
	return true
}

// hasTagCaseInsensitive checks if any tag matches the given name and value,
// using case-insensitive comparison.
func hasTagCaseInsensitive(tags []core.Tag, name, value string) bool {
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) && strings.EqualFold(t.Value, value) {
			return true
		}
	}
	return false
}

// matchOwnershipByComment returns true when every value in the ownership
// map can be found inside the comment string using case-insensitive regex.
func matchOwnershipByComment(comment string, ownership map[string]string) bool {
	if comment == "" || len(ownership) == 0 {
		return false
	}
	for _, val := range ownership {
		if val == "" {
			continue
		}
		pattern := "(?i)" + regexp.QuoteMeta(val)
		matched, err := regexp.MatchString(pattern, comment)
		if err != nil || !matched {
			return false
		}
	}
	return true
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

