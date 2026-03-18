package runtimectx

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/containerrt"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// DefaultResolver collects runtime context from the host environment.
type DefaultResolver struct {
	Logger           *logging.Logger
	ExplicitNodeID   string
	PublicIPv4URLs   []string
	PublicIPv6URLs   []string
	HTTPTimeout      time.Duration
}

// NewDefaultResolver returns a DefaultResolver with sensible defaults.
func NewDefaultResolver(logger *logging.Logger, explicitNodeID string) *DefaultResolver {
	return &DefaultResolver{
		Logger:         logger,
		ExplicitNodeID: explicitNodeID,
		PublicIPv4URLs: []string{
			"https://api.ipify.org",
			"https://ifconfig.me/ip",
			"https://checkip.amazonaws.com",
		},
		PublicIPv6URLs: []string{
			"https://api6.ipify.org",
			"https://ifconfig.co/ip",
		},
		HTTPTimeout: 10 * time.Second,
	}
}

// Resolve collects the full runtime snapshot.
func (r *DefaultResolver) Resolve(ctx context.Context) (Snapshot, error) {
	snap := Snapshot{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Environment:  collectEnvironment(),
	}

	hostname, err := os.Hostname()
	if err != nil {
		r.Logger.Warning(fmt.Sprintf("Failed to resolve hostname: %s", err))
	} else {
		snap.Hostname = hostname
	}

	snap.NodeID = r.resolveNodeID(snap.Hostname)
	snap.InterfaceAddresses = r.collectInterfaceAddresses()

	// Auto-detect container runtime networks and exclude their CIDRs from
	// RFC1918/CGNAT address selection so we never pick a Docker/Podman
	// bridge address (e.g. 172.17.0.1) over a real host address.
	detector := containerrt.NewDetector(r.Logger)
	var excludedPrefixes []netip.Prefix
	if detector.HasRuntime() {
		for _, cidr := range detector.ExcludedCIDRs(ctx) {
			if p, err := netip.ParsePrefix(cidr); err == nil {
				excludedPrefixes = append(excludedPrefixes, p)
			}
		}
		if len(excludedPrefixes) > 0 {
			r.Logger.Debug(fmt.Sprintf("Container runtime: excluding %d CIDR(s) from address selection", len(excludedPrefixes)))
		}
	}
	// Store the detector on the snapshot so the reconciliation pipeline
	// can use it for container discovery without re-probing sockets.
	snap.ContainerDetector = detector

	snap.PublicIPv4 = r.fetchPublicIP(ctx, r.PublicIPv4URLs, "IPv4")
	snap.PublicIPv6 = r.fetchPublicIP(ctx, r.PublicIPv6URLs, "IPv6")
	snap.RFC1918IPv4 = findFirstMatchingAddress(snap.InterfaceAddresses, isRFC1918, excludedPrefixes)
	snap.CGNATIPv4 = findFirstMatchingAddress(snap.InterfaceAddresses, isCGNAT, excludedPrefixes)

	r.Logger.Debug(fmt.Sprintf("Runtime context resolved: hostname=%s nodeID=%s os=%s arch=%s publicIPv4=%s publicIPv6=%s rfc1918=%s cgnat=%s",
		snap.Hostname, snap.NodeID, snap.OS, snap.Architecture, snap.PublicIPv4, snap.PublicIPv6, snap.RFC1918IPv4, snap.CGNATIPv4))

	return snap, nil
}

// resolveNodeID follows spec §11.4: explicit → machine UUID → hostname.
func (r *DefaultResolver) resolveNodeID(hostname string) string {
	if r.ExplicitNodeID != "" {
		r.Logger.Debug(fmt.Sprintf("Using explicit node ID: %s", r.ExplicitNodeID))
		return r.ExplicitNodeID
	}
	if id := readMachineID(); id != "" {
		r.Logger.Debug(fmt.Sprintf("Using machine UUID as node ID: %s", id))
		return id
	}
	r.Logger.Debug(fmt.Sprintf("Using hostname as node ID fallback: %s", hostname))
	return hostname
}

// collectInterfaceAddresses gathers all unicast addresses grouped by interface.
func (r *DefaultResolver) collectInterfaceAddresses() map[string][]string {
	result := map[string][]string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		r.Logger.Warning(fmt.Sprintf("Failed to enumerate network interfaces: %s", err))
		return result
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			r.Logger.Warning(fmt.Sprintf("Failed to get addresses for interface %s: %s", iface.Name, err))
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			result[iface.Name] = append(result[iface.Name], ip.String())
		}
	}
	return result
}

// fetchPublicIP tries each URL in order and returns the first successful response.
func (r *DefaultResolver) fetchPublicIP(ctx context.Context, urls []string, label string) string {
	client := &http.Client{Timeout: r.HTTPTimeout}
	for _, url := range urls {
		reqCtx, cancel := context.WithTimeout(ctx, r.HTTPTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			r.Logger.Trace(fmt.Sprintf("Public %s fetch from %s failed: %s", label, url, err))
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		cancel()
		if resp.StatusCode == http.StatusOK {
			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				r.Logger.Debug(fmt.Sprintf("Resolved public %s: %s (from %s)", label, ip, url))
				return ip
			}
		}
	}
	r.Logger.Debug(fmt.Sprintf("No public %s address resolved", label))
	return ""
}

