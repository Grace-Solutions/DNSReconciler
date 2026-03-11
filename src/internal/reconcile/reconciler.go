// Package reconcile implements the per-record reconciliation engine (§21).
package reconcile

import (
	"context"
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/address"
	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
)

// ActionKind describes the outcome of a reconciliation pass.
type ActionKind string

const (
	ActionNoop   ActionKind = "noop"
	ActionCreate ActionKind = "create"
	ActionUpdate ActionKind = "update"
	ActionDelete ActionKind = "delete"
	ActionSkip   ActionKind = "skip"
)

// Result captures the outcome of a single record reconciliation.
type Result struct {
	RecordID string
	Action   ActionKind
	Error    error
}

// Stats aggregates reconciliation counts.
type Stats struct {
	Created int
	Updated int
	Deleted int
	Skipped int
	Errors  int
}

// Reconciler orchestrates per-record reconciliation per §21.2.
type Reconciler struct {
	Logger          *logging.Logger
	Providers       map[string]core.Provider
	AddressResolver *address.DefaultResolver
	Snapshot        runtimectx.Snapshot
	GlobalSources   []config.AddressSource
	DryRun          bool
}

// ReconcileAll runs per-record reconciliation for all resolved templates (§21.1 steps 9-10).
func (r *Reconciler) ReconcileAll(ctx context.Context, templates []config.RecordTemplate, st *state.File) (Stats, []Result) {
	var stats Stats
	var results []Result

	for _, tmpl := range templates {
		if tmpl.Enabled != nil && !*tmpl.Enabled {
			r.Logger.Debug(fmt.Sprintf("Record %s is disabled, skipping", tmpl.RecordID))
			stats.Skipped++
			results = append(results, Result{RecordID: tmpl.RecordID, Action: ActionSkip})
			continue
		}

		result := r.reconcileOne(ctx, tmpl, st)
		results = append(results, result)
		if result.Error != nil {
			stats.Errors++
		} else {
			switch result.Action {
			case ActionCreate:
				stats.Created++
			case ActionUpdate:
				stats.Updated++
			case ActionDelete:
				stats.Deleted++
			case ActionSkip:
				stats.Skipped++
			case ActionNoop:
				// nothing
			}
		}
	}

	r.Logger.Information(fmt.Sprintf("Reconciliation complete: created=%d updated=%d deleted=%d skipped=%d errors=%d",
		stats.Created, stats.Updated, stats.Deleted, stats.Skipped, stats.Errors))
	return stats, results
}

