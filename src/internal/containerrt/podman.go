package containerrt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// Well-known Podman socket paths probed in order.
// The user-level socket uses XDG_RUNTIME_DIR.
func podmanSocketPaths() []string {
	paths := []string{
		"/run/podman/podman.sock",
		"/var/run/podman/podman.sock",
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "podman", "podman.sock"))
	}
	return paths
}

// PodmanClient communicates with the Podman API over a Unix socket.
// Podman's REST API is Docker-compatible, so we reuse the same response types.
type PodmanClient struct {
	logger     *logging.Logger
	httpClient *http.Client
	socketPath string
}

// NewPodmanClient probes for a Podman socket and returns a client, or nil if
// Podman is not available.
func NewPodmanClient(logger *logging.Logger) *PodmanClient {
	for _, sock := range podmanSocketPaths() {
		if isSocket(sock) {
			logger.Debug(fmt.Sprintf("Container runtime: Podman socket found at %s", sock))
			transport := &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", sock)
				},
			}
			return &PodmanClient{
				logger:     logger,
				socketPath: sock,
				httpClient: &http.Client{Transport: transport},
			}
		}
	}
	return nil
}

func (p *PodmanClient) Runtime() Runtime { return RuntimePodman }

func (p *PodmanClient) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	body, err := p.get(ctx, "/networks")
	if err != nil {
		return nil, fmt.Errorf("podman list networks: %w", err)
	}
	defer body.Close()

	var raw []dockerNetwork
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("podman decode networks: %w", err)
	}

	var networks []NetworkInfo
	for _, n := range raw {
		info := NetworkInfo{ID: n.ID, Name: n.Name, Driver: n.Driver}
		for _, cfg := range n.IPAM.Config {
			if cfg.Subnet != "" {
				info.Subnets = append(info.Subnets, cfg.Subnet)
			}
		}
		networks = append(networks, info)
	}
	p.logger.Information(fmt.Sprintf("Container runtime: Podman returned %d networks", len(networks)))
	return networks, nil
}

func (p *PodmanClient) ExcludedCIDRs(ctx context.Context) ([]string, error) {
	networks, err := p.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}
	var cidrs []string
	for _, n := range networks {
		if IsExcludedDriver(n.Driver) {
			cidrs = append(cidrs, n.Subnets...)
			if len(n.Subnets) > 0 {
				p.logger.Debug(fmt.Sprintf("Container runtime: excluding Podman network %q (%s) subnets: %v", n.Name, n.Driver, n.Subnets))
			}
		}
	}
	return cidrs, nil
}

func (p *PodmanClient) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	body, err := p.get(ctx, "/containers/json")
	if err != nil {
		return nil, fmt.Errorf("podman list containers: %w", err)
	}
	defer body.Close()

	var raw []dockerContainer
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("podman decode containers: %w", err)
	}

	var containers []ContainerInfo
	for _, c := range raw {
		info := ContainerInfo{
			ID:       c.ID,
			Name:     cleanContainerName(c.Names),
			Image:    c.Image,
			State:    c.State,
			Labels:   c.Labels,
			Networks: make(map[string]ContainerNetwork),
		}
		if c.NetworkSettings.Networks != nil {
			for name, n := range c.NetworkSettings.Networks {
				info.Networks[name] = ContainerNetwork{
					NetworkID:   n.NetworkID,
					NetworkName: name,
					IPAddress:   n.IPAddress,
					MacAddress:  n.MacAddress,
					Gateway:     n.Gateway,
				}
			}
		}
		// Fetch hostname via inspect.
		if hostname, err := p.inspectHostname(ctx, c.ID); err == nil {
			info.Hostname = hostname
		}

		containers = append(containers, info)
	}
	p.logger.Information(fmt.Sprintf("Container runtime: Podman returned %d running containers", len(containers)))
	return containers, nil
}

// inspectHostname retrieves the container hostname from /containers/{id}/json.
func (p *PodmanClient) inspectHostname(ctx context.Context, id string) (string, error) {
	body, err := p.get(ctx, "/containers/"+id+"/json")
	if err != nil {
		return "", err
	}
	defer body.Close()
	var insp dockerInspect
	if err := json.NewDecoder(body).Decode(&insp); err != nil {
		return "", err
	}
	return insp.Config.Hostname, nil
}

func (p *PodmanClient) get(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}
	return resp.Body, nil
}

