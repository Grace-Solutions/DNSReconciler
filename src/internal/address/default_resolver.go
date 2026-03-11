package address

import (
	"context"
	"fmt"
	"net"
	"sort"

	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
)

// DefaultResolver evaluates address sources in ascending priority order (§12.1).
type DefaultResolver struct {
	Logger *logging.Logger
}

// NewDefaultResolver creates a DefaultResolver.
func NewDefaultResolver(logger *logging.Logger) *DefaultResolver {
	return &DefaultResolver{Logger: logger}
}

// Resolve evaluates sources in priority order and returns the first usable address.
func (r *DefaultResolver) Resolve(
	_ context.Context,
	snap runtimectx.Snapshot,
	sources []config.AddressSource,
	ipFamily string,
) (Result, error) {
	// Sort sources by ascending priority (§12.1: 1 = first choice).
	sorted := make([]config.AddressSource, len(sources))
	copy(sorted, sources)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	for _, src := range sorted {
		if !src.Enabled {
			continue
		}
		addr := r.resolveSource(snap, src)
		if addr == "" {
			r.Logger.Trace(fmt.Sprintf("Address source %s (priority %d) yielded no result", src.Type, src.Priority))
			continue
		}
		ip := net.ParseIP(addr)
		if ip == nil {
			r.Logger.Trace(fmt.Sprintf("Address source %s returned unparseable address %q", src.Type, addr))
			continue
		}
		if !matchesFamily(ip, ipFamily) {
			r.Logger.Trace(fmt.Sprintf("Address %s from %s does not match family %s", addr, src.Type, ipFamily))
			continue
		}
		if !matchesCIDRConstraints(ip, src.AllowRanges, src.DenyRanges) {
			r.Logger.Trace(fmt.Sprintf("Address %s from %s excluded by CIDR constraints", addr, src.Type))
			continue
		}
		r.Logger.Debug(fmt.Sprintf("Selected address %s from source %s (priority %d)", addr, src.Type, src.Priority))
		return Result{Source: src, Address: addr}, nil
	}
	return Result{}, fmt.Errorf("no usable address found from %d source(s)", len(sources))
}

// resolveSource returns the raw address for a given source type.
func (r *DefaultResolver) resolveSource(snap runtimectx.Snapshot, src config.AddressSource) string {
	switch src.Type {
	case "publicIPv4":
		return snap.PublicIPv4
	case "publicIPv6":
		return snap.PublicIPv6
	case "rfc1918IPv4":
		return snap.RFC1918IPv4
	case "cgnatIPv4":
		return snap.CGNATIPv4
	case "interfaceIPv4":
		return findInterfaceAddress(snap.InterfaceAddresses, src.InterfaceName, false)
	case "interfaceIPv6":
		return findInterfaceAddress(snap.InterfaceAddresses, src.InterfaceName, true)
	case "explicitIPv4", "explicitIPv6":
		return src.ExplicitValue
	default:
		r.Logger.Warning(fmt.Sprintf("Unknown address source type %q", src.Type))
		return ""
	}
}

// findInterfaceAddress returns the first address on the named interface
// matching the requested IP version. If interfaceName is empty, searches all interfaces.
func findInterfaceAddress(ifaces map[string][]string, name string, wantIPv6 bool) string {
	check := func(addrs []string) string {
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}
			isV6 := ip.To4() == nil
			if isV6 == wantIPv6 {
				return addr
			}
		}
		return ""
	}
	if name != "" {
		if addrs, ok := ifaces[name]; ok {
			return check(addrs)
		}
		return ""
	}
	for _, addrs := range ifaces {
		if result := check(addrs); result != "" {
			return result
		}
	}
	return ""
}

// matchesFamily checks if the IP matches the requested family filter.
func matchesFamily(ip net.IP, family string) bool {
	switch family {
	case "ipv4", "IPv4":
		return ip.To4() != nil
	case "ipv6", "IPv6":
		return ip.To4() == nil
	default:
		return true // "dual" or empty = accept either
	}
}

// matchesCIDRConstraints enforces allow/deny CIDR rules (§12.3).
func matchesCIDRConstraints(ip net.IP, allow, deny []string) bool {
	if len(deny) > 0 {
		for _, cidr := range deny {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if network.Contains(ip) {
				return false
			}
		}
	}
	if len(allow) > 0 {
		for _, cidr := range allow {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if network.Contains(ip) {
				return true
			}
		}
		return false // allow list is non-empty but IP didn't match any
	}
	return true
}

