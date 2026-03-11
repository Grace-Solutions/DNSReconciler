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
	if nameTypeCandidate != nil {
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

// coreOwnershipKeys lists the ownership keys that are always generated
// by buildOwnershipFilter. Used to distinguish user-supplied keys from
// system keys during the truncation pass.
var coreOwnershipKeys = map[string]bool{
	"managed-by":         true,
	"node-id":            true,
	"record-template-id": true,
}

// buildOwnershipComment creates a JSON-structured comment that embeds
// ownership metadata alongside an optional user-supplied note or custom
// key-value pairs.
//
// If the user comment is a JSON object (single quotes are accepted and
// converted to double quotes for convenience), its keys are merged into
// the ownership map. This lets users add custom metadata without escaping:
//
//	config:  "comment": "{'hostname': '${HOSTNAME}', 'nodeId': '${NODE_ID}'}"
//	result:  {"managed-by":"dnsreconciler","node-id":"abc","record-template-id":"xyz","hostname":"myserver","nodeId":"n1"}
//
// If the user comment is plain text, it is stored under the "note" key:
//
//	config:  "comment": "managed by automation"
//	result:  {"managed-by":"dnsreconciler","node-id":"abc","record-template-id":"xyz","note":"managed by automation"}
//
// The final string is capped at maxCommentLength (100) characters. When
// the full JSON exceeds this limit, fields are removed in priority order:
//  1. "note" (plain-text fallback)
//  2. User-supplied keys (anything not in coreOwnershipKeys)
//  3. "managed-by" (always "dnsreconciler" — least unique)
//  4. "record-template-id"
//
// "node-id" is never removed — it is the most unique identifier.
func buildOwnershipComment(userComment string, ownership map[string]string) string {
	obj := make(map[string]string, len(ownership)+4)
	for k, v := range ownership {
		obj[k] = v
	}
	if userComment != "" {
		mergeUserComment(obj, userComment)
	}
	return marshalAndTruncate(obj)
}

// marshalAndTruncate serialises obj as JSON. If the result exceeds
// maxCommentLength, it removes keys one at a time in priority order
// until the string fits.
func marshalAndTruncate(obj map[string]string) string {
	data, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	if len(data) <= maxCommentLength {
		return string(data)
	}

	// Removal priority (first removed → least important).
	// 1. "note"
	if _, ok := obj["note"]; ok {
		delete(obj, "note")
		if s := tryMarshal(obj); len(s) <= maxCommentLength {
			return s
		}
	}
	// 2. User-supplied keys (non-core, non-note).
	for k := range obj {
		if coreOwnershipKeys[k] {
			continue
		}
		delete(obj, k)
		if s := tryMarshal(obj); len(s) <= maxCommentLength {
			return s
		}
	}
	// 3. "managed-by"
	delete(obj, "managed-by")
	if s := tryMarshal(obj); len(s) <= maxCommentLength {
		return s
	}
	// 4. "record-template-id"
	delete(obj, "record-template-id")
	return tryMarshal(obj) // node-id alone should always fit.
}

// tryMarshal is a convenience wrapper that returns "" on error.
func tryMarshal(obj map[string]string) string {
	data, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(data)
}

// mergeUserComment attempts to parse the user comment as a JSON object.
// Single quotes are converted to double quotes first so users can avoid
// escaping in their JSON config files. If parsing succeeds, keys are
// merged into obj (user keys do NOT overwrite ownership keys). If parsing
// fails, the raw text is stored under the "note" key.
func mergeUserComment(obj map[string]string, comment string) {
	normalized := strings.ReplaceAll(comment, "'", "\"")
	var userObj map[string]string
	if err := json.Unmarshal([]byte(normalized), &userObj); err == nil {
		for k, v := range userObj {
			if _, exists := obj[k]; !exists {
				obj[k] = v
			}
		}
		return
	}
	obj["note"] = comment
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

