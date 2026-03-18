package runtimectx

import (
	"net/netip"
	"os"
	"runtime"
	"strings"
)

// collectEnvironment returns all environment variables as a map.
func collectEnvironment() map[string]string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

// readMachineID attempts to read a stable machine UUID from the OS.
// Spec §11.4: machine UUID is preferred over hostname.
func readMachineID() string {
	switch runtime.GOOS {
	case "linux":
		for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if data, err := os.ReadFile(path); err == nil {
				id := strings.TrimSpace(string(data))
				if id != "" {
					return id
				}
			}
		}
	case "darwin":
		// macOS IOPlatformUUID requires ioreg; skip for now, fallback to hostname.
	case "windows":
		// Windows machine GUID is in the registry; skip for now, fallback to hostname.
	}
	return ""
}

// RFC1918 private address prefixes (parsed once at init).
var rfc1918Prefixes []netip.Prefix

// CGNAT address prefix (parsed once at init).
var cgnatPrefix netip.Prefix

func init() {
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		rfc1918Prefixes = append(rfc1918Prefixes, netip.MustParsePrefix(cidr))
	}
	cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")
}

// isRFC1918 returns true if addr falls in an RFC 1918 range.
func isRFC1918(addr netip.Addr) bool {
	for _, p := range rfc1918Prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// isCGNAT returns true if addr falls in the CGNAT range (100.64.0.0/10).
func isCGNAT(addr netip.Addr) bool {
	return cgnatPrefix.Contains(addr)
}

// findFirstMatchingAddress scans all interface addresses and returns the first
// IPv4 address matching the predicate, skipping any addresses in excludedCIDRs.
func findFirstMatchingAddress(ifaces map[string][]string, predicate func(netip.Addr) bool, excludedCIDRs []netip.Prefix) string {
	for _, addrs := range ifaces {
		for _, a := range addrs {
			addr, err := netip.ParseAddr(a)
			if err != nil {
				continue
			}
			// Only consider IPv4 for RFC1918/CGNAT.
			if !addr.Is4() {
				continue
			}
			if isExcludedByPrefix(addr, excludedCIDRs) {
				continue
			}
			if predicate(addr) {
				return addr.String()
			}
		}
	}
	return ""
}

// isExcludedByPrefix returns true if addr falls within any of the given prefixes.
func isExcludedByPrefix(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

