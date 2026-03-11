package address

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
)

func testLogger() *logging.Logger {
	return logging.New(&bytes.Buffer{}, logging.LevelTrace)
}

func TestResolve_PublicIPv4FirstPriority(t *testing.T) {
	snap := runtimectx.Snapshot{
		PublicIPv4:  "203.0.113.5",
		RFC1918IPv4: "192.168.1.10",
	}
	sources := []config.AddressSource{
		{Priority: 1, Type: "publicIPv4", Enabled: true},
		{Priority: 2, Type: "rfc1918IPv4", Enabled: true},
	}
	r := NewDefaultResolver(testLogger())
	result, err := r.Resolve(context.Background(), snap, sources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Address != "203.0.113.5" {
		t.Errorf("expected 203.0.113.5; got %s", result.Address)
	}
}

func TestResolve_FallbackToSecondPriority(t *testing.T) {
	snap := runtimectx.Snapshot{
		PublicIPv4:  "",
		RFC1918IPv4: "192.168.1.10",
	}
	sources := []config.AddressSource{
		{Priority: 1, Type: "publicIPv4", Enabled: true},
		{Priority: 2, Type: "rfc1918IPv4", Enabled: true},
	}
	r := NewDefaultResolver(testLogger())
	result, err := r.Resolve(context.Background(), snap, sources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Address != "192.168.1.10" {
		t.Errorf("expected 192.168.1.10; got %s", result.Address)
	}
}

func TestResolve_NoUsableAddress(t *testing.T) {
	snap := runtimectx.Snapshot{}
	sources := []config.AddressSource{
		{Priority: 1, Type: "publicIPv4", Enabled: true},
	}
	r := NewDefaultResolver(testLogger())
	_, err := r.Resolve(context.Background(), snap, sources, "")
	if err == nil {
		t.Fatal("expected error when no address is available")
	}
}

func TestResolve_DisabledSourceSkipped(t *testing.T) {
	snap := runtimectx.Snapshot{
		PublicIPv4:  "203.0.113.5",
		RFC1918IPv4: "10.0.0.1",
	}
	sources := []config.AddressSource{
		{Priority: 1, Type: "publicIPv4", Enabled: false},
		{Priority: 2, Type: "rfc1918IPv4", Enabled: true},
	}
	r := NewDefaultResolver(testLogger())
	result, err := r.Resolve(context.Background(), snap, sources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Address != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1; got %s", result.Address)
	}
}

func TestMatchesCIDRConstraints_AllowList(t *testing.T) {
	ip := net.ParseIP("100.64.0.5")
	if !matchesCIDRConstraints(ip, []string{"100.64.0.0/10"}, nil) {
		t.Error("expected IP to match allow range")
	}
	if matchesCIDRConstraints(ip, []string{"10.0.0.0/8"}, nil) {
		t.Error("expected IP to be excluded when not in allow range")
	}
}

func TestMatchesCIDRConstraints_DenyList(t *testing.T) {
	ip := net.ParseIP("10.0.0.5")
	if matchesCIDRConstraints(ip, nil, []string{"10.0.0.0/8"}) {
		t.Error("expected IP to be denied")
	}
}

func TestMatchesFamily(t *testing.T) {
	v4 := net.ParseIP("192.168.1.1")
	v6 := net.ParseIP("2001:db8::1")
	if !matchesFamily(v4, "ipv4") {
		t.Error("IPv4 should match ipv4 family")
	}
	if matchesFamily(v4, "ipv6") {
		t.Error("IPv4 should not match ipv6 family")
	}
	if !matchesFamily(v6, "ipv6") {
		t.Error("IPv6 should match ipv6 family")
	}
	if matchesFamily(v6, "ipv4") {
		t.Error("IPv6 should not match ipv4 family")
	}
	if !matchesFamily(v4, "dual") {
		t.Error("IPv4 should match dual family")
	}
	if !matchesFamily(v6, "") {
		t.Error("IPv6 should match empty (any) family")
	}
}

func TestResolve_InterfaceSource(t *testing.T) {
	snap := runtimectx.Snapshot{
		InterfaceAddresses: map[string][]string{
			"tailscale0": {"100.64.1.5"},
		},
	}
	sources := []config.AddressSource{
		{Priority: 1, Type: "interfaceIPv4", Enabled: true, InterfaceName: "tailscale0",
			AllowRanges: []string{"100.64.0.0/10"}},
	}
	r := NewDefaultResolver(testLogger())
	result, err := r.Resolve(context.Background(), snap, sources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Address != "100.64.1.5" {
		t.Errorf("expected 100.64.1.5; got %s", result.Address)
	}
}

func TestResolve_ExplicitSource(t *testing.T) {
	snap := runtimectx.Snapshot{}
	sources := []config.AddressSource{
		{Priority: 1, Type: "explicitIPv4", Enabled: true, ExplicitValue: "1.2.3.4"},
	}
	r := NewDefaultResolver(testLogger())
	result, err := r.Resolve(context.Background(), snap, sources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Address != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4; got %s", result.Address)
	}
}

