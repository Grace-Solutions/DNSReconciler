// Package cleanup implements graceful shutdown record deletion (§26).
package cleanup

import (
	"context"
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
)

// Cleaner handles deleting owned records on graceful shutdown.
type Cleaner struct {
	Logger    *logging.Logger
	Providers map[string]core.Provider
	Store     state.Store
}

// Run deletes all records tracked in local state from their respective providers (§26.1).
// It iterates pending cleanup items first, then falls back to state records.
// On completion, it clears the state to reflect that records are removed.
func (c *Cleaner) Run(ctx context.Context) error {
	st, err := c.Store.Load(ctx)
	if err != nil {
		return fmt.Errorf("cleanup: load state: %w", err)
	}

	var totalErrors int

	// §26.1: delete pending cleanup items (records that were being tracked for removal)
	for _, item := range st.PendingCleanup {
		if err := c.deleteCleanupItem(ctx, item); err != nil {
			c.Logger.Error(fmt.Sprintf("Cleanup: failed to delete pending item %s/%s: %s", item.Name, item.Type, err))
			totalErrors++
		}
	}

	// §26.1: delete all records we own from state
	for templateID, recState := range st.Records {
		if recState.ProviderRecordID == "" {
			continue
		}
		if err := c.deleteStateRecord(ctx, templateID, recState); err != nil {
			c.Logger.Error(fmt.Sprintf("Cleanup: failed to delete record %s: %s", templateID, err))
			totalErrors++
		}
	}

	// Clear state after cleanup
	st.Records = map[string]state.RecordState{}
	st.PendingCleanup = nil
	if err := c.Store.Save(ctx, st); err != nil {
		return fmt.Errorf("cleanup: save cleared state: %w", err)
	}

	if totalErrors > 0 {
		c.Logger.Warning(fmt.Sprintf("Cleanup completed with %d error(s)", totalErrors))
	} else {
		c.Logger.Information("Cleanup: all owned records deleted successfully")
	}
	return nil
}

// deleteCleanupItem deletes a single pending cleanup record from its provider.
func (c *Cleaner) deleteCleanupItem(ctx context.Context, item state.CleanupItem) error {
	prov, ok := c.Providers[item.Provider]
	if !ok {
		return fmt.Errorf("provider %q not registered", item.Provider)
	}

	record := core.Record{
		Provider:         item.Provider,
		Zone:             item.Zone,
		Name:             item.Name,
		Type:             item.Type,
		ProviderRecordID: item.ProviderRecordID,
		RecordTemplateID: item.RecordTemplateID,
	}

	c.Logger.Information(fmt.Sprintf("Cleanup: deleting %s %s (provider: %s)", item.Type, item.Name, item.Provider))
	return prov.DeleteRecord(ctx, record)
}

// deleteStateRecord deletes a record tracked in state from its provider.
// It extracts the provider name from the template ID convention or state metadata.
func (c *Cleaner) deleteStateRecord(ctx context.Context, templateID string, recState state.RecordState) error {
	// Try each provider to find one that can handle this record
	for provName, prov := range c.Providers {
		record := core.Record{
			Provider:         provName,
			ProviderRecordID: recState.ProviderRecordID,
			RecordTemplateID: templateID,
		}
		err := prov.DeleteRecord(ctx, record)
		if err == nil {
			c.Logger.Information(fmt.Sprintf("Cleanup: deleted record %s via %s", templateID, provName))
			return nil
		}
		c.Logger.Debug(fmt.Sprintf("Cleanup: provider %s could not delete %s: %s", provName, templateID, err))
	}
	return fmt.Errorf("no provider could delete record %s", templateID)
}

