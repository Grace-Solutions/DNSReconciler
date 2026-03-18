// Package containerrt detects container runtimes (Docker, Podman) and provides
// two capabilities:
//   1. Network exclusion — enumerate container-managed network CIDRs so the
//      address resolver can skip virtual bridge addresses (Phase 1).
//   2. Service discovery — find running containers on externally-routable
//      networks (IPVLAN/MACVLAN) for automatic DNS registration (Phase 2).
package containerrt

import (
	"context"
	"net/netip"
)

// Runtime identifies a container engine.
type Runtime string

const (
	RuntimeDocker Runtime = "docker"
	RuntimePodman Runtime = "podman"
)

// NetworkInfo describes a container-managed network and its subnets.
type NetworkInfo struct {
	ID      string
	Name    string
	Driver  string   // bridge, macvlan, ipvlan, overlay, host, null …
	Subnets []string // CIDR notation, e.g. "172.17.0.0/16"
}

// ContainerInfo describes a running container with its network attachments.
type ContainerInfo struct {
	ID        string
	Name      string            // human-readable name (leading "/" stripped)
	Image     string
	State     string            // running, exited, …
	Labels    map[string]string
	Networks  map[string]ContainerNetwork // key = network name
}

// ContainerNetwork holds a container's attachment to a single network.
type ContainerNetwork struct {
	NetworkID   string
	NetworkName string
	Driver      string // populated after enrichment
	IPAddress   string
	MacAddress  string
	Gateway     string
}

// Client is the interface for querying a container runtime.
type Client interface {
	// Runtime returns which engine this client talks to.
	Runtime() Runtime

	// ListNetworks returns all networks managed by the runtime.
	ListNetworks(ctx context.Context) ([]NetworkInfo, error)

	// ExcludedCIDRs returns the CIDRs of networks whose addresses should be
	// excluded from host address selection. This includes bridge, overlay,
	// and any other non-externally-routable network types.
	ExcludedCIDRs(ctx context.Context) ([]string, error)

	// ListContainers returns running containers with their network attachments.
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
}

// IsExcludedDriver returns true for network drivers that are never externally
// routable and whose CIDRs should be excluded from host address selection.
func IsExcludedDriver(driver string) bool {
	switch driver {
	case "bridge", "overlay", "null", "host", "none":
		return true
	default:
		return false
	}
}

// IsRoutableDriver returns true for network drivers that attach containers
// directly to an external L2 segment. Both IPVLAN and MACVLAN are supported:
//   - IPVLAN L2 is preferred in cloud/virtualised environments (no promiscuous mode needed).
//   - MACVLAN works well on bare-metal where the NIC can be set to promiscuous mode.
func IsRoutableDriver(driver string) bool {
	return driver == "ipvlan" || driver == "macvlan"
}

// ContainsCIDR returns true when addr falls within any of the given CIDRs.
func ContainsCIDR(addr netip.Addr, cidrs []string) bool {
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

