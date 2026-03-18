package runtimectx

import (
	"net/netip"
	"testing"
)

func TestIsRFC1918(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.32.0.1", false},
		{"192.168.1.1", true},
		{"192.168.0.0", true},
		{"8.8.8.8", false},
		{"100.64.0.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("invalid test IP: %s", tt.ip)
			}
			got := isRFC1918(addr)
			if got != tt.want {
				t.Errorf("isRFC1918(%s) = %v; want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsCGNAT(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"10.0.0.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("invalid test IP: %s", tt.ip)
			}
			got := isCGNAT(addr)
			if got != tt.want {
				t.Errorf("isCGNAT(%s) = %v; want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestFindFirstMatchingAddress(t *testing.T) {
	ifaces := map[string][]string{
		"eth0": {"203.0.113.5", "192.168.1.10"},
		"eth1": {"10.0.0.5"},
	}
	got := findFirstMatchingAddress(ifaces, isRFC1918, nil)
	if got == "" {
		t.Fatal("expected to find an RFC1918 address")
	}
	addr, err := netip.ParseAddr(got)
	if err != nil {
		t.Fatalf("unparseable result: %s", got)
	}
	if !isRFC1918(addr) {
		t.Errorf("findFirstMatchingAddress returned %s which is not RFC1918", got)
	}
}

func TestFindFirstMatchingAddress_NoMatch(t *testing.T) {
	ifaces := map[string][]string{
		"eth0": {"203.0.113.5"},
	}
	got := findFirstMatchingAddress(ifaces, isRFC1918, nil)
	if got != "" {
		t.Errorf("expected empty string; got %s", got)
	}
}

func TestFindFirstMatchingAddress_ExcludesCIDRs(t *testing.T) {
	ifaces := map[string][]string{
		"docker0": {"172.17.0.1"},
		"eth0":    {"172.16.16.39"},
	}
	excluded := []netip.Prefix{netip.MustParsePrefix("172.17.0.0/16")}
	got := findFirstMatchingAddress(ifaces, isRFC1918, excluded)
	if got == "172.17.0.1" {
		t.Error("should have excluded Docker bridge address 172.17.0.1")
	}
	if got == "" {
		t.Fatal("expected to find 172.16.16.39 after excluding Docker bridge")
	}
	if got != "172.16.16.39" {
		t.Errorf("expected 172.16.16.39; got %s", got)
	}
}

func TestCollectEnvironment(t *testing.T) {
	env := collectEnvironment()
	// PATH should always exist on all platforms.
	found := false
	for key := range env {
		if key == "PATH" || key == "Path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected PATH or Path in environment")
	}
}

