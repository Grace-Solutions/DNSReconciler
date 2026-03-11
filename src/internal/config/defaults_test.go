package config

import (
	"testing"
)

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func TestMergeDefaults_ProviderFillsEmpty(t *testing.T) {
	prov := &ProviderEntry{
		ID:      "cf-1",
		Type:    "cloudflare",
		TTL:     intPtr(120),
		Proxied: boolPtr(false),
		Comment: "managed",
	}
	rec := RecordTemplate{RecordID: "r1", ProviderID: "cf-1", Zone: "example.com", Type: "A", Name: "a.example.com"}
	merged := MergeDefaults(rec, prov)
	if merged.Enabled == nil || *merged.Enabled != true {
		t.Error("expected Enabled=true from built-in defaults")
	}
	if merged.Ownership != "perNode" {
		t.Errorf("expected ownership=perNode from built-in; got %s", merged.Ownership)
	}
	if merged.TTL == nil || *merged.TTL != 120 {
		t.Error("expected TTL=120 from provider")
	}
	if merged.Proxied == nil || *merged.Proxied != false {
		t.Error("expected Proxied=false from provider")
	}
	if merged.Comment != "managed" {
		t.Errorf("expected comment=managed; got %s", merged.Comment)
	}
}

func TestMergeDefaults_PerRecordWins(t *testing.T) {
	prov := &ProviderEntry{
		ID:      "cf-1",
		Type:    "cloudflare",
		TTL:     intPtr(120),
		Proxied: boolPtr(false),
	}
	rec := RecordTemplate{
		RecordID: "r1", ProviderID: "cf-1", Zone: "example.com",
		Type: "A", Name: "a.example.com",
		TTL:     intPtr(60),
		Proxied: boolPtr(true),
	}
	merged := MergeDefaults(rec, prov)
	if *merged.TTL != 60 {
		t.Errorf("expected per-record TTL=60; got %d", *merged.TTL)
	}
	if *merged.Proxied != true {
		t.Error("expected per-record Proxied=true to win")
	}
}

func TestMergeDefaults_BuiltInFallback(t *testing.T) {
	// No provider — built-in defaults should apply
	rec := RecordTemplate{RecordID: "r1", ProviderID: "cf-1", Zone: "example.com", Type: "A", Name: "a.example.com"}
	merged := MergeDefaults(rec, nil)
	if merged.Enabled == nil || *merged.Enabled != true {
		t.Error("expected Enabled=true from built-in defaults")
	}
	if merged.Ownership != "perNode" {
		t.Errorf("expected ownership=perNode from built-in; got %s", merged.Ownership)
	}
	if merged.TTL == nil || *merged.TTL != 120 {
		t.Error("expected TTL=120 from built-in defaults")
	}
}

func TestMergeDefaults_ZoneInheritedFromProvider(t *testing.T) {
	prov := &ProviderEntry{ID: "cf", Type: "cloudflare", Zone: "example.com"}
	rec := RecordTemplate{RecordID: "r1", ProviderID: "cf", Type: "A", Name: "a.example.com"}
	merged := MergeDefaults(rec, prov)
	if merged.Zone != "example.com" {
		t.Errorf("expected zone=example.com from provider; got %s", merged.Zone)
	}
}

func TestMergeAllDefaults(t *testing.T) {
	cfg := &Config{
		Providers: []ProviderEntry{
			{ID: "cf", Type: "cloudflare", TTL: intPtr(300)},
		},
		Records: []RecordTemplate{
			{RecordID: "r1", ProviderID: "cf", Type: "A", Name: "a"},
			{RecordID: "r2", ProviderID: "cf", Type: "A", Name: "b", TTL: intPtr(60)},
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
	prov := &ProviderEntry{
		ID:   "cf",
		Type: "cloudflare",
		Tags: []Tag{{Name: "managed-by", Value: "dns-reconciler"}},
	}
	rec := RecordTemplate{RecordID: "r1", ProviderID: "cf", Type: "A", Name: "a"}
	merged := MergeDefaults(rec, prov)
	if len(merged.Tags) != 1 || merged.Tags[0].Value != "dns-reconciler" {
		t.Error("expected tags to be inherited from provider")
	}
}

func TestMergeDefaults_PerRecordTagsWin(t *testing.T) {
	prov := &ProviderEntry{
		ID:   "cf",
		Type: "cloudflare",
		Tags: []Tag{{Name: "managed-by", Value: "dns-reconciler"}},
	}
	rec := RecordTemplate{
		RecordID: "r1", ProviderID: "cf", Type: "A", Name: "a",
		Tags: []Tag{{Name: "custom", Value: "val"}},
	}
	merged := MergeDefaults(rec, prov)
	if len(merged.Tags) != 1 || merged.Tags[0].Name != "custom" {
		t.Error("expected per-record tags to win over provider")
	}
}

