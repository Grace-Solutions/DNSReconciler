package reconcile

import (
	"context"
	"fmt"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/expansion"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
)

// reconcileOne implements §21.2 for a single record.
func (r *Reconciler) reconcileOne(ctx context.Context, tmpl config.RecordTemplate, st *state.File) Result {
	recordID := tmpl.RecordID
	r.Logger.Debug(fmt.Sprintf("Reconciling record %s", recordID))

	// §21.2 step 1: determine applicable provider (looked up by providerId)
	provider, ok := r.Providers[tmpl.ProviderID]
	if !ok {
		err := fmt.Errorf("provider %q not registered", tmpl.ProviderID)
		r.Logger.Error(fmt.Sprintf("Record %s: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionSkip, Error: err}
	}

	// §21.2 steps 2-3: determine address source policy and resolve address
	sources := r.GlobalSources
	if tmpl.AddressSelection != nil {
		if tmpl.AddressSelection.UseGlobalDefaults != nil && !*tmpl.AddressSelection.UseGlobalDefaults {
			sources = tmpl.AddressSelection.Sources
		}
	}

	addrResult, err := r.AddressResolver.Resolve(ctx, r.Snapshot, sources, tmpl.IPFamily)
	if err != nil {
		// §21.2 step 4 / §22.3: log warning and skip
		r.Logger.Warning(fmt.Sprintf("No usable address source found for record %s: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionSkip}
	}

	// §21.2 step 5: construct desired record via variable expansion
	rv := expansion.RecordVars{
		SelectedIPv4: selectByFamily(addrResult.Address, "ipv4"),
		SelectedIPv6: selectByFamily(addrResult.Address, "ipv6"),
		ServiceName:  tmpl.RecordID,
		Zone:         tmpl.Zone,
		RecordID:     tmpl.RecordID,
	}
	expCtx := expansion.BuildContext(r.Snapshot, rv)

	name, err := expansion.MustExpand(tmpl.Name, expCtx)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Record %s: name expansion failed: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionSkip, Error: err}
	}
	content := addrResult.Address
	if tmpl.Content != "" {
		content, err = expansion.MustExpand(tmpl.Content, expCtx)
		if err != nil {
			r.Logger.Error(fmt.Sprintf("Record %s: content expansion failed: %s", recordID, err))
			return Result{RecordID: recordID, Action: ActionSkip, Error: err}
		}
	}
	comment := ""
	if tmpl.Comment != "" {
		res := expansion.Expand(tmpl.Comment, expCtx)
		comment = res.Value
	}

	ttl := 120
	if tmpl.TTL != nil {
		ttl = *tmpl.TTL
	}
	proxied := false
	if tmpl.Proxied != nil {
		proxied = *tmpl.Proxied
	}

	tags := make([]core.Tag, len(tmpl.Tags))
	for i, t := range tmpl.Tags {
		vr := expansion.Expand(t.Value, expCtx)
		tags[i] = core.Tag{Name: t.Name, Value: vr.Value}
	}

	// Enforce capability gating: strip tags if the provider does not support them.
	caps := provider.Capabilities()
	if !caps.SupportsStructuredTags && len(tags) > 0 {
		r.Logger.Debug(fmt.Sprintf("Record %s: provider does not support tags, stripping %d tag(s)", recordID, len(tags)))
		tags = nil
	}

	// Log the fully expanded record state for visibility.
	r.Logger.Information(fmt.Sprintf("Record %s: expanded → Type=%s Name=%s Content=%s Zone=%s TTL=%d Tags=%d",
		recordID, tmpl.Type, name, content, tmpl.Zone, ttl, len(tags)))

	desired := core.Record{
		Provider:         tmpl.ProviderID,
		Zone:             tmpl.Zone,
		Type:             tmpl.Type,
		Name:             name,
		Content:          content,
		TTL:              ttl,
		Enabled:          true,
		Proxied:          proxied,
		Tags:             tags,
		OwnershipMode:    tmpl.Ownership,
		RecordTemplateID: tmpl.RecordID,
	}

	// §21.2 steps 6-9: query provider, identify owned records, compare
	filter := buildOwnershipFilter(desired, r.Snapshot.NodeID)

	// Build a JSON-structured comment embedding ownership metadata.
	// The user's comment template (if any) is stored under the "note" key.
	desired.Comment = buildOwnershipComment(comment, filter.Ownership)
	r.Logger.Information(fmt.Sprintf("Record %s: comment (%d chars) → %s", recordID, len(desired.Comment), desired.Comment))
	desired.DesiredFingerprint = fingerprint(desired)
	existing, err := provider.ListRecords(ctx, filter)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Record %s: provider list failed: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionSkip, Error: err}
	}

	// §21.2 step 10: perform action
	return r.performAction(ctx, provider, desired, existing, recordID, st, filter.Ownership)
}

// performAction determines and executes the correct action.
func (r *Reconciler) performAction(ctx context.Context, provider core.Provider, desired core.Record, existing []core.Record, recordID string, st *state.File, ownership map[string]string) Result {
	owned, matchMethod := findOwnedRecord(existing, desired, ownership)

	if owned != nil {
		r.Logger.Information(fmt.Sprintf("Record %s: ownership matched via %s (existing comment: %q)", recordID, matchMethod, owned.Comment))
		return r.reconcileExisting(ctx, provider, desired, owned, recordID, st)
	}

	// No existing record found — attempt to create.
	r.Logger.Information(fmt.Sprintf("Record %s: creating", recordID))
	if r.DryRun {
		r.Logger.Information(fmt.Sprintf("Record %s: [dry-run] would create", recordID))
		return Result{RecordID: recordID, Action: ActionCreate}
	}
	created, err := provider.CreateRecord(ctx, desired)
	if err != nil {
		// Safety net: if the provider reports the record already exists
		// (e.g. Cloudflare error 81058), handle it as a reconciliation
		// candidate instead of a hard error.
		if isDuplicateRecordError(err) {
			r.Logger.Information(fmt.Sprintf("Record %s: duplicate detected, reconciling existing record", recordID))
			return r.handleDuplicateOnCreate(ctx, provider, desired, recordID, st, ownership)
		}
		r.Logger.Error(fmt.Sprintf("Record %s: create failed: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionCreate, Error: err}
	}
	updateState(st, recordID, created, desired.DesiredFingerprint, desired.Content)
	return Result{RecordID: recordID, Action: ActionCreate}
}

// reconcileExisting handles the case where an existing record was found.
// If the content matches, it logs and skips. If content differs, it updates.
func (r *Reconciler) reconcileExisting(ctx context.Context, provider core.Provider, desired core.Record, owned *core.Record, recordID string, st *state.File) Result {
	// Content-based check: if the record already has the correct content,
	// check whether the comment/metadata also matches. If the comment is
	// stale (e.g. pre-JSON format), update the record to stamp the new
	// structured comment so that future passes match via comment-json.
	if strings.EqualFold(owned.Content, desired.Content) {
		if owned.Comment == desired.Comment {
			r.Logger.Information(fmt.Sprintf("Record %s: already exists with correct content and comment, skipping", recordID))
			updateState(st, recordID, *owned, desired.DesiredFingerprint, desired.Content)
			return Result{RecordID: recordID, Action: ActionNoop}
		}
		// Content matches but comment is stale — update to stamp ownership metadata.
		r.Logger.Information(fmt.Sprintf("Record %s: content matches but comment differs, updating comment", recordID))
		r.Logger.Debug(fmt.Sprintf("Record %s: comment have=%q want=%q", recordID, owned.Comment, desired.Comment))
		desired.ProviderRecordID = owned.ProviderRecordID
		if r.DryRun {
			r.Logger.Information(fmt.Sprintf("Record %s: [dry-run] would update comment", recordID))
			return Result{RecordID: recordID, Action: ActionUpdate}
		}
		updated, err := provider.UpdateRecord(ctx, desired)
		if err != nil {
			r.Logger.Error(fmt.Sprintf("Record %s: comment update failed: %s", recordID, err))
			return Result{RecordID: recordID, Action: ActionUpdate, Error: err}
		}
		updateState(st, recordID, updated, desired.DesiredFingerprint, desired.Content)
		return Result{RecordID: recordID, Action: ActionUpdate}
	}

	// Content differs — update the record.
	r.Logger.Information(fmt.Sprintf("Record %s: content differs (have=%s want=%s), updating", recordID, owned.Content, desired.Content))
	desired.ProviderRecordID = owned.ProviderRecordID
	if r.DryRun {
		r.Logger.Information(fmt.Sprintf("Record %s: [dry-run] would update", recordID))
		return Result{RecordID: recordID, Action: ActionUpdate}
	}
	updated, err := provider.UpdateRecord(ctx, desired)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Record %s: update failed: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionUpdate, Error: err}
	}
	updateState(st, recordID, updated, desired.DesiredFingerprint, desired.Content)
	return Result{RecordID: recordID, Action: ActionUpdate}
}

// handleDuplicateOnCreate re-queries the provider after a duplicate-create
// error, finds the existing record, and reconciles it (skip or update).
func (r *Reconciler) handleDuplicateOnCreate(ctx context.Context, provider core.Provider, desired core.Record, recordID string, st *state.File, ownership map[string]string) Result {
	filter := core.RecordFilter{Zone: desired.Zone, Name: desired.Name, Type: desired.Type}
	records, err := provider.ListRecords(ctx, filter)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Record %s: re-query after duplicate failed: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionCreate, Error: err}
	}
	owned, matchMethod := findOwnedRecord(records, desired, ownership)
	if owned != nil {
		r.Logger.Information(fmt.Sprintf("Record %s: duplicate re-query matched via %s", recordID, matchMethod))
	} else if len(records) > 0 {
		// Use first result as candidate.
		owned = &records[0]
		r.Logger.Information(fmt.Sprintf("Record %s: duplicate re-query using first result (no ownership match)", recordID))
	}
	if owned == nil {
		err := fmt.Errorf("record exists at provider but could not be located on re-query")
		r.Logger.Error(fmt.Sprintf("Record %s: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionCreate, Error: err}
	}
	return r.reconcileExisting(ctx, provider, desired, owned, recordID, st)
}

// isDuplicateRecordError returns true when the error indicates that
// the provider already has an identical record (e.g. Cloudflare 81058).
func isDuplicateRecordError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate")
}

