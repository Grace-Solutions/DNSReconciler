package reconcile

import (
	"crypto/sha256"
	"encoding/json"
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
// The ownership map is derived from the desired record's comment field:
// if the comment is valid JSON, its key-value pairs become the ownership
// map used for matching. If the comment is empty or not JSON, no
// ownership keys are set and matching falls through to name+type.
func buildOwnershipFilter(desired core.Record) core.RecordFilter {
	ownership := parseCommentAsOwnership(desired.Comment)
	return core.RecordFilter{
		Zone:      desired.Zone,
		Name:      desired.Name,
		Type:      desired.Type,
		Ownership: ownership,
	}
}

// parseCommentAsOwnership attempts to parse the comment as a JSON object
// and returns its key-value pairs as the ownership map. Returns nil if
// the comment is empty or not valid JSON.
func parseCommentAsOwnership(comment string) map[string]string {
	if comment == "" {
		return nil
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(comment), &obj); err != nil {
		return nil
	}
	return obj
}

// Ownership match method constants returned by findOwnedRecord.
const (
	MatchNone         = ""
	MatchTags         = "tags"
	MatchCommentJSON  = "comment-json"
	MatchCommentRegex = "comment-regex"
	MatchNameType     = "name+type"
)

// findOwnedRecord locates the record we own among the existing set.
// It matches by name+type and then checks ownership via tags or comment
// using case-insensitive matching. It returns the matched record and a
// string describing which mechanism produced the match.
//
// Strategy (in priority order):
//  1. If the existing record has tags, check whether every ownership
//     key-value pair appears in the tags (case-insensitive).
//  2. If tag matching fails (or no tags present), try parsing the
//     comment as JSON and match key-value pairs (comment-json).
//  3. If JSON parsing fails, fall back to case-insensitive regex
//     searching for each ownership value (comment-regex).
//  4. If neither tag nor comment matched, fall back to name+type match.
//     This ensures that a record already present on the provider (even
//     without ownership metadata) is treated as a reconciliation
//     candidate rather than triggering a duplicate-create error.
func findOwnedRecord(existing []core.Record, desired core.Record, ownership map[string]string) (*core.Record, string) {
	var nameTypeCandidate *core.Record
	for i, rec := range existing {
		if !strings.EqualFold(rec.Name, desired.Name) || !strings.EqualFold(rec.Type, desired.Type) {
			continue
		}
		if matchOwnershipByTags(rec.Tags, ownership) {
			return &existing[i], MatchTags
		}
		if method := matchOwnershipByCommentDetailed(rec.Comment, ownership); method != MatchNone {
			return &existing[i], method
		}
		// Track first name+type match as fallback candidate.
		if nameTypeCandidate == nil {
			nameTypeCandidate = &existing[i]
		}
	}
	// Only fall back to name+type when there are NO ownership keys.
	// When ownership keys are present (e.g. nodeId), each node must match
	// exclusively via its own metadata. This enables DNS round-robin: multiple
	// nodes can register A records for the same FQDN without clobbering each
	// other's entries.
	if nameTypeCandidate != nil && len(ownership) == 0 {
		return nameTypeCandidate, MatchNameType
	}
	return nil, MatchNone
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

// maxCommentLength is the maximum number of characters allowed in the
// comment field. Providers like Cloudflare enforce a 100-character limit.
const maxCommentLength = 100

// buildOwnershipComment normalises the user-supplied comment for use as
// the DNS record's comment field. No system keys are injected — the
// comment is exactly what the user specifies.
//
// Single quotes are converted to double quotes so users can write JSON
// inside their JSON config without escaping:
//
//	config:  "comment": "{'hostname': '${HOSTNAME}', 'nodeId': '${NODE_ID}'}"
//	result:  {"hostname":"myserver","nodeId":"ba9298a1c5b2..."}
//
// If the result exceeds maxCommentLength (100 chars) it is truncated.
// Plain-text comments are used as-is (truncated if necessary).
func buildOwnershipComment(userComment string) string {
	if userComment == "" {
		return ""
	}
	// Normalise single quotes → double quotes for JSON convenience.
	normalised := strings.ReplaceAll(userComment, "'", "\"")

	// If it's valid JSON, re-marshal for a canonical form.
	var obj map[string]string
	if err := json.Unmarshal([]byte(normalised), &obj); err == nil {
		data, err := json.Marshal(obj)
		if err == nil {
			normalised = string(data)
		}
	}

	// Enforce provider length limit.
	if len(normalised) > maxCommentLength {
		normalised = normalised[:maxCommentLength]
	}
	return normalised
}

// matchOwnershipByCommentDetailed checks whether the comment contains the
// required ownership key-value pairs and returns the specific mechanism
// that matched (MatchCommentJSON, MatchCommentRegex, or MatchNone).
func matchOwnershipByCommentDetailed(comment string, ownership map[string]string) string {
	if comment == "" || len(ownership) == 0 {
		return MatchNone
	}
	if matchOwnershipByCommentJSON(comment, ownership) {
		return MatchCommentJSON
	}
	if matchOwnershipByCommentRegex(comment, ownership) {
		return MatchCommentRegex
	}
	return MatchNone
}

// matchOwnershipByCommentJSON parses the comment as a JSON object and
// checks ownership key-value pairs (case-insensitive). Keys that are
// present in the JSON must match their expected value; keys that are
// absent are skipped (they may have been removed by the 100-char
// truncation pass). At least one key must match to avoid false positives.
func matchOwnershipByCommentJSON(comment string, ownership map[string]string) bool {
	var obj map[string]string
	if err := json.Unmarshal([]byte(comment), &obj); err != nil {
		return false
	}
	matched := 0
	for key, val := range ownership {
		// Look for this ownership key in the JSON object.
		for k, v := range obj {
			if strings.EqualFold(k, key) {
				// Key is present — value must match.
				if !strings.EqualFold(v, val) {
					return false
				}
				matched++
				break
			}
		}
		// Key absent from JSON → skip (may have been truncated).
	}
	return matched > 0
}

// matchOwnershipByCommentRegex searches for each ownership value in the
// comment text using case-insensitive literal matching. This is a backward-
// compatible fallback for records that predate the JSON comment format.
func matchOwnershipByCommentRegex(comment string, ownership map[string]string) bool {
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

