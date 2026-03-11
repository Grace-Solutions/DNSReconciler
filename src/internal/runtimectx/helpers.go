package runtimectx

import (
	"net"
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

// RFC1918 private address ranges.
var rfc1918Ranges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// CGNAT address range (100.64.0.0/10).
var cgnatRange = "100.64.0.0/10"

// isRFC1918 returns true if the address falls in an RFC1918 range.
func isRFC1918(ip net.IP) bool {
	for _, cidr := range rfc1918Ranges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// isCGNAT returns true if the address falls in the CGNAT range (100.64.0.0/10).
func isCGNAT(ip net.IP) bool {
	_, network, _ := net.ParseCIDR(cgnatRange)
	return network != nil && network.Contains(ip)
}

// findFirstMatchingAddress scans all interface addresses and returns the first
// IPv4 address matching the predicate.
func findFirstMatchingAddress(ifaces map[string][]string, predicate func(net.IP) bool) string {
	for _, addrs := range ifaces {
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}
			// Only consider IPv4 for RFC1918/CGNAT.
			if ip.To4() == nil {
				continue
			}
			if predicate(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

