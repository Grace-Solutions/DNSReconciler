package reconcile

import (
	"bytes"
	"context"
	"testing"

	"github.com/gracesolutions/dns-automatic-updater/internal/address"
	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
	"github.com/gracesolutions/dns-automatic-updater/internal/state"
)

// stubProvider implements core.Provider for testing.
type stubProvider struct {
	name     string
	records  []core.Record
	created  []core.Record
	updated  []core.Record
	deleted  []core.Record
}

func (s *stubProvider) Name() string                                     { return s.name }
func (s *stubProvider) ValidateConfig(map[string]any) error              { return nil }
func (s *stubProvider) Capabilities() core.ProviderCapabilities          { return core.ProviderCapabilities{} }
func (s *stubProvider) ListRecords(_ context.Context, _ core.RecordFilter) ([]core.Record, error) {
	return s.records, nil
}
func (s *stubProvider) CreateRecord(_ context.Context, r core.Record) (core.Record, error) {
	r.ProviderRecordID = "new-id-1"
	s.created = append(s.created, r)
	return r, nil
}
func (s *stubProvider) UpdateRecord(_ context.Context, r core.Record) (core.Record, error) {
	s.updated = append(s.updated, r)
	return r, nil
}
func (s *stubProvider) DeleteRecord(_ context.Context, r core.Record) error {
	s.deleted = append(s.deleted, r)
	return nil
}

func testLogger() *logging.Logger {
	return logging.New(&bytes.Buffer{}, logging.LevelTrace)
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func TestReconcileAll_CreateNewRecord(t *testing.T) {
	prov := &stubProvider{name: "testprov"}
	logger := testLogger()
	snap := runtimectx.Snapshot{
		Hostname:     "node1",
		NodeID:       "node1",
		OS:           "linux",
		Architecture: "amd64",
		PublicIPv4:   "203.0.113.5",
	}
	st := state.File{Records: map[string]state.RecordState{}}

	r := Reconciler{
		Logger:          logger,
		Providers:       map[string]core.Provider{"testprov": prov},
		AddressResolver: address.NewDefaultResolver(logger),
		Snapshot:        snap,
		GlobalSources: []config.AddressSource{
			{Priority: 1, Type: "publicIPv4", Enabled: true},
		},
	}

	templates := []config.RecordTemplate{{
		RecordID: "test-rec", Enabled: boolPtr(true), ProviderID: "testprov",
		Zone: "example.com", Type: "A", Name: "a.example.com",
		Content: "${SELECTED_IPV4}", Ownership: "perNode", TTL: intPtr(120),
	}}

	stats, results := r.ReconcileAll(context.Background(), templates, &st)
	if stats.Created != 1 {
		t.Errorf("expected 1 created; got %d", stats.Created)
	}
	if len(results) != 1 || results[0].Action != ActionCreate {
		t.Errorf("expected ActionCreate; got %v", results)
	}
	if len(prov.created) != 1 {
		t.Errorf("expected provider.CreateRecord called once; got %d", len(prov.created))
	}
}

func TestReconcileAll_SkipDisabledRecord(t *testing.T) {
	logger := testLogger()
	snap := runtimectx.Snapshot{PublicIPv4: "203.0.113.5"}
	st := state.File{Records: map[string]state.RecordState{}}

	r := Reconciler{
		Logger:          logger,
		Providers:       map[string]core.Provider{},
		AddressResolver: address.NewDefaultResolver(logger),
		Snapshot:        snap,
	}

	templates := []config.RecordTemplate{{
		RecordID: "disabled-rec", Enabled: boolPtr(false), ProviderID: "testprov",
		Zone: "example.com", Type: "A", Name: "a.example.com",
		Content: "${SELECTED_IPV4}",
	}}

	stats, _ := r.ReconcileAll(context.Background(), templates, &st)
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped; got %d", stats.Skipped)
	}
}

func TestReconcileAll_SkipMissingProvider(t *testing.T) {
	logger := testLogger()
	snap := runtimectx.Snapshot{PublicIPv4: "203.0.113.5"}
	st := state.File{Records: map[string]state.RecordState{}}

	r := Reconciler{
		Logger:          logger,
		Providers:       map[string]core.Provider{},
		AddressResolver: address.NewDefaultResolver(logger),
		Snapshot:        snap,
		GlobalSources: []config.AddressSource{
			{Priority: 1, Type: "publicIPv4", Enabled: true},
		},
	}

	templates := []config.RecordTemplate{{
		RecordID: "no-prov", Enabled: boolPtr(true), ProviderID: "missing",
		Zone: "example.com", Type: "A", Name: "a.example.com",
		Content: "${SELECTED_IPV4}", TTL: intPtr(120),
	}}

	stats, results := r.ReconcileAll(context.Background(), templates, &st)
	if stats.Errors != 1 {
		t.Errorf("expected 1 error; got %d", stats.Errors)
	}
	if results[0].Error == nil {
		t.Error("expected error for missing provider")
	}
}

func TestReconcileAll_DryRunDoesNotCallProvider(t *testing.T) {
	prov := &stubProvider{name: "testprov"}
	logger := testLogger()
	snap := runtimectx.Snapshot{PublicIPv4: "203.0.113.5", Hostname: "n1", NodeID: "n1"}
	st := state.File{Records: map[string]state.RecordState{}}

	r := Reconciler{
		Logger: logger, Providers: map[string]core.Provider{"testprov": prov},
		AddressResolver: address.NewDefaultResolver(logger), Snapshot: snap,
		GlobalSources: []config.AddressSource{{Priority: 1, Type: "publicIPv4", Enabled: true}},
		DryRun: true,
	}

	templates := []config.RecordTemplate{{
		RecordID: "dr", Enabled: boolPtr(true), ProviderID: "testprov",
		Zone: "example.com", Type: "A", Name: "a.example.com",
		Content: "${SELECTED_IPV4}", TTL: intPtr(60),
	}}

	r.ReconcileAll(context.Background(), templates, &st)
	if len(prov.created) != 0 {
		t.Error("dry-run should not call provider.CreateRecord")
	}
}

