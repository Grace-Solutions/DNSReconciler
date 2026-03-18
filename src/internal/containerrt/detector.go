package containerrt

import (
	"context"
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// Detector probes for available container runtimes and provides aggregated
// network exclusion and container discovery across all detected engines.
type Detector struct {
	logger  *logging.Logger
	clients []Client
}

// NewDetector probes for Docker and Podman sockets and returns a Detector
// wrapping all discovered runtimes. If none are found the Detector is still
// usable — it simply returns empty results.
func NewDetector(logger *logging.Logger) *Detector {
	var clients []Client

	if dc := NewDockerClient(logger); dc != nil {
		clients = append(clients, dc)
	}
	if pc := NewPodmanClient(logger); pc != nil {
		clients = append(clients, pc)
	}

	if len(clients) == 0 {
		logger.Debug("Container runtime: no Docker or Podman socket detected")
	}
	return &Detector{logger: logger, clients: clients}
}

// HasRuntime returns true if at least one container runtime was detected.
func (d *Detector) HasRuntime() bool { return len(d.clients) > 0 }

// Runtimes returns the names of detected container engines.
func (d *Detector) Runtimes() []Runtime {
	var runtimes []Runtime
	for _, c := range d.clients {
		runtimes = append(runtimes, c.Runtime())
	}
	return runtimes
}

// ExcludedCIDRs aggregates excluded CIDRs from all detected runtimes.
// Errors are logged and skipped — a failing runtime does not block others.
func (d *Detector) ExcludedCIDRs(ctx context.Context) []string {
	var all []string
	for _, c := range d.clients {
		cidrs, err := c.ExcludedCIDRs(ctx)
		if err != nil {
			d.logger.Warning(fmt.Sprintf("Container runtime: failed to get excluded CIDRs from %s: %s", c.Runtime(), err))
			continue
		}
		all = append(all, cidrs...)
	}
	return all
}

// AllNetworks returns every network from all detected runtimes.
func (d *Detector) AllNetworks(ctx context.Context) []NetworkInfo {
	var all []NetworkInfo
	for _, c := range d.clients {
		networks, err := c.ListNetworks(ctx)
		if err != nil {
			d.logger.Warning(fmt.Sprintf("Container runtime: failed to list networks from %s: %s", c.Runtime(), err))
			continue
		}
		all = append(all, networks...)
	}
	return all
}

// AllContainers returns every running container from all detected runtimes.
func (d *Detector) AllContainers(ctx context.Context) []ContainerInfo {
	var all []ContainerInfo
	for _, c := range d.clients {
		containers, err := c.ListContainers(ctx)
		if err != nil {
			d.logger.Warning(fmt.Sprintf("Container runtime: failed to list containers from %s: %s", c.Runtime(), err))
			continue
		}
		all = append(all, containers...)
	}
	return all
}

// RoutableContainer is a container that has at least one IP on a routable
// (IPVLAN or MACVLAN) network. The RoutableIP is the address on that network.
type RoutableContainer struct {
	ContainerInfo
	RoutableIP      string
	RoutableNetwork string // network name
}

// RoutableContainers discovers containers attached to IPVLAN or MACVLAN
// networks and returns one entry per routable IP. Containers on non-routable
// networks only are excluded. If labelFilter is non-empty, only containers
// whose labels are a superset of the filter are included.
func (d *Detector) RoutableContainers(ctx context.Context, labelFilter map[string]string) []RoutableContainer {
	// Build network-ID → driver map.
	networkDrivers := make(map[string]string)
	for _, net := range d.AllNetworks(ctx) {
		networkDrivers[net.ID] = net.Driver
	}

	var result []RoutableContainer
	for _, c := range d.AllContainers(ctx) {
		if !matchesLabels(c.Labels, labelFilter) {
			continue
		}
		for netName, net := range c.Networks {
			driver := networkDrivers[net.NetworkID]
			if !IsRoutableDriver(driver) {
				continue
			}
			if net.IPAddress == "" {
				continue
			}
			rc := RoutableContainer{
				ContainerInfo:   c,
				RoutableIP:      net.IPAddress,
				RoutableNetwork: netName,
			}
			d.logger.Debug(fmt.Sprintf("Container runtime: routable container %q on %s (%s) → %s",
				c.Name, netName, driver, net.IPAddress))
			result = append(result, rc)
		}
	}
	return result
}

// matchesLabels returns true if container labels contain all key-value pairs
// in the filter. An empty filter matches everything.
func matchesLabels(labels, filter map[string]string) bool {
	for k, v := range filter {
		if labels[k] != v {
			return false
		}
	}
	return true
}

