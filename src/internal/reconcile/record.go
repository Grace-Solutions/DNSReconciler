package reconcile

import (
	"context"
	"fmt"

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
		Comment:          comment,
		Tags:             tags,
		OwnershipMode:    tmpl.Ownership,
		RecordTemplateID: tmpl.RecordID,
	}
	desired.DesiredFingerprint = fingerprint(desired)

	// §21.2 steps 6-9: query provider, identify owned records, compare
	filter := buildOwnershipFilter(desired, r.Snapshot.NodeID)
	existing, err := provider.ListRecords(ctx, filter)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Record %s: provider list failed: %s", recordID, err))
		return Result{RecordID: recordID, Action: ActionSkip, Error: err}
	}

	// §21.2 step 10: perform action
	return r.performAction(ctx, provider, desired, existing, recordID, st)
}

// performAction determines and executes the correct action.
func (r *Reconciler) performAction(ctx context.Context, provider core.Provider, desired core.Record, existing []core.Record, recordID string, st *state.File) Result {
	owned := findOwnedRecord(existing, desired)

	if owned == nil {
		// Create
		r.Logger.Information(fmt.Sprintf("Record %s: creating", recordID))
		if r.DryRun {
			r.Logger.Information(fmt.Sprintf("Record %s: [dry-run] would create", recordID))
			return Result{RecordID: recordID, Action: ActionCreate}
		}
		created, err := provider.CreateRecord(ctx, desired)
		if err != nil {
			r.Logger.Error(fmt.Sprintf("Record %s: create failed: %s", recordID, err))
			return Result{RecordID: recordID, Action: ActionCreate, Error: err}
		}
		updateState(st, recordID, created, desired.DesiredFingerprint, desired.Content)
		return Result{RecordID: recordID, Action: ActionCreate}
	}

	// Compare fingerprint
	if desired.DesiredFingerprint == st.Records[recordID].DesiredFingerprint {
		r.Logger.Debug(fmt.Sprintf("Record %s: no change", recordID))
		return Result{RecordID: recordID, Action: ActionNoop}
	}

	// Update
	r.Logger.Information(fmt.Sprintf("Record %s: updating", recordID))
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

