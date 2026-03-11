package config

import (
	"testing"
)

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func TestMergeDefaults_GlobalFillsEmpty(t *testing.T) {
	gd := RecordDefaults{
		Enabled:   boolPtr(true),
		Ownership: "perNode",
		TTL:       intPtr(120),
		Proxied:   boolPtr(false),
		Comment:   "managed",
	}
	rec := RecordTemplate{ID: "r1", Provider: "cloudflare", Zone: "example.com", Type: "A", Name: "a.example.com"}
	merged := MergeDefaults(rec, gd, nil)
	if merged.Enabled == nil || *merged.Enabled != true {
		t.Error("expected Enabled=true from global defaults")
	}
	if merged.Ownership != "perNode" {
		t.Errorf("expected ownership=perNode; got %s", merged.Ownership)
	}
	if merged.TTL == nil || *merged.TTL != 120 {
		t.Error("expected TTL=120 from global defaults")
	}
	if merged.Proxied == nil || *merged.Proxied != false {
		t.Error("expected Proxied=false from global defaults")
	}
	if merged.Comment != "managed" {
		t.Errorf("expected comment=managed; got %s", merged.Comment)
	}
}

func TestMergeDefaults_PerRecordWins(t *testing.T) {
	gd := RecordDefaults{
		TTL:     intPtr(120),
		Proxied: boolPtr(false),
	}
	rec := RecordTemplate{
		ID: "r1", Provider: "cloudflare", Zone: "example.com",
		Type: "A", Name: "a.example.com",
		TTL:     intPtr(60),
		Proxied: boolPtr(true),
	}
	merged := MergeDefaults(rec, gd, nil)
	if *merged.TTL != 60 {
		t.Errorf("expected per-record TTL=60; got %d", *merged.TTL)
	}
	if *merged.Proxied != true {
		t.Error("expected per-record Proxied=true to win")
	}
}

func TestMergeDefaults_ProviderZoneFallback(t *testing.T) {
	pd := map[string]map[string]any{
		"cloudflare": {"zone": "cf.example.com"},
	}
	rec := RecordTemplate{ID: "r1", Provider: "cloudflare", Type: "A", Name: "a.cf.example.com"}
	merged := MergeDefaults(rec, RecordDefaults{}, pd)
	if merged.Zone != "cf.example.com" {
		t.Errorf("expected zone=cf.example.com from provider defaults; got %s", merged.Zone)
	}
}

func TestMergeDefaults_PerRecordZoneWins(t *testing.T) {
	pd := map[string]map[string]any{
		"cloudflare": {"zone": "cf.example.com"},
	}
	rec := RecordTemplate{ID: "r1", Provider: "cloudflare", Zone: "custom.com", Type: "A", Name: "a.custom.com"}
	merged := MergeDefaults(rec, RecordDefaults{}, pd)
	if merged.Zone != "custom.com" {
		t.Errorf("expected per-record zone=custom.com; got %s", merged.Zone)
	}
}

func TestMergeAllDefaults(t *testing.T) {
	cfg := &Config{
		Defaults: RecordDefaults{TTL: intPtr(300)},
		Records: []RecordTemplate{
			{ID: "r1", Provider: "cf", Type: "A", Name: "a"},
			{ID: "r2", Provider: "cf", Type: "A", Name: "b", TTL: intPtr(60)},
		},
	}
	merged := MergeAllDefaults(cfg)
	if len(merged) != 2 {
		t.Fatalf("expected 2 records; got %d", len(merged))
	}
	if *merged[0].TTL != 300 {
		t.Errorf("record 0 expected TTL=300; got %d", *merged[0].TTL)
	}
	if *merged[1].TTL != 60 {
		t.Errorf("record 1 expected TTL=60; got %d", *merged[1].TTL)
	}
}

func TestMergeDefaults_TagsInherited(t *testing.T) {
	gd := RecordDefaults{
		Tags: []Tag{{Name: "managed-by", Value: "dns-reconciler"}},
	}
	rec := RecordTemplate{ID: "r1", Provider: "cf", Type: "A", Name: "a"}
	merged := MergeDefaults(rec, gd, nil)
	if len(merged.Tags) != 1 || merged.Tags[0].Value != "dns-reconciler" {
		t.Error("expected tags to be inherited from global defaults")
	}
}

func TestMergeDefaults_PerRecordTagsWin(t *testing.T) {
	gd := RecordDefaults{
		Tags: []Tag{{Name: "managed-by", Value: "dns-reconciler"}},
	}
	rec := RecordTemplate{
		ID: "r1", Provider: "cf", Type: "A", Name: "a",
		Tags: []Tag{{Name: "custom", Value: "val"}},
	}
	merged := MergeDefaults(rec, gd, nil)
	if len(merged.Tags) != 1 || merged.Tags[0].Name != "custom" {
		t.Error("expected per-record tags to win over global defaults")
	}
}

