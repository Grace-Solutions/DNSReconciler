package containerrt

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
// networks only are excluded.
//
// matchFields controls which container metadata fields the include/exclude
// regex patterns are evaluated against. Supported values:
//
//	"auto"          — expands to containername + hostname (default)
//	"containername" — container name
//	"hostname"      — container hostname (from inspect)
//	"image"         — container image
//	"containerid"   — full container ID
//
// If matchFields is empty or contains only "auto", it defaults to
// ["containername", "hostname"]. Exclude takes precedence over include.
func (d *Detector) RoutableContainers(ctx context.Context, include, exclude, matchFields []string) []RoutableContainer {
	// Compile regex patterns once.
	includeRx := compilePatterns(include, d.logger)
	excludeRx := compilePatterns(exclude, d.logger)

	// Resolve match fields.
	fields := resolveMatchFields(matchFields)

	// Build network-ID → driver map.
	networkDrivers := make(map[string]string)
	for _, net := range d.AllNetworks(ctx) {
		networkDrivers[net.ID] = net.Driver
	}

	var result []RoutableContainer
	for _, c := range d.AllContainers(ctx) {
		if !matchesFilters(c, fields, includeRx, excludeRx) {
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

// resolveMatchFields expands "auto" and normalises field names. If the input
// is empty, it defaults to ["containername", "hostname"].
func resolveMatchFields(raw []string) []string {
	if len(raw) == 0 {
		return []string{"containername", "hostname"}
	}
	var fields []string
	for _, f := range raw {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "auto" {
			fields = append(fields, "containername", "hostname")
		} else {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return []string{"containername", "hostname"}
	}
	return fields
}

// fieldValues returns the values to match against for the given container
// and list of field names.
func fieldValues(c ContainerInfo, fields []string) []string {
	var vals []string
	for _, f := range fields {
		switch f {
		case "containername":
			vals = append(vals, c.Name)
		case "hostname":
			vals = append(vals, c.Hostname)
		case "image":
			vals = append(vals, c.Image)
		case "containerid":
			vals = append(vals, c.ID)
		}
	}
	return vals
}

// compilePatterns compiles a list of regex pattern strings. Invalid patterns
// are logged and skipped.
func compilePatterns(patterns []string, logger *logging.Logger) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		rx, err := regexp.Compile(p)
		if err != nil {
			logger.Warning(fmt.Sprintf("Container filter: invalid regex %q: %s", p, err))
			continue
		}
		compiled = append(compiled, rx)
	}
	return compiled
}

// matchesFilters returns true if the container passes the include/exclude
// filter. Regex patterns are evaluated against the values of the specified
// match fields. Exclude always takes precedence. If no include patterns are
// provided, all containers are included by default.
func matchesFilters(c ContainerInfo, fields []string, include, exclude []*regexp.Regexp) bool {
	vals := fieldValues(c, fields)

	// Check exclude first — if any field value matches any exclude pattern, reject.
	for _, rx := range exclude {
		for _, v := range vals {
			if rx.MatchString(v) {
				return false
			}
		}
	}
	// If no include patterns, everything passes.
	if len(include) == 0 {
		return true
	}
	// At least one include must match at least one field value.
	for _, rx := range include {
		for _, v := range vals {
			if rx.MatchString(v) {
				return true
			}
		}
	}
	return false
}

