package containerrt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// Well-known socket paths probed in order.
var dockerSocketPaths = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
}

// DockerClient communicates with the Docker Engine API over a Unix socket.
type DockerClient struct {
	logger     *logging.Logger
	httpClient *http.Client
	socketPath string
}

// NewDockerClient probes for a Docker socket and returns a client, or nil if
// Docker is not available.
func NewDockerClient(logger *logging.Logger) *DockerClient {
	for _, sock := range dockerSocketPaths {
		if isSocket(sock) {
			logger.Debug(fmt.Sprintf("Container runtime: Docker socket found at %s", sock))
			transport := &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", sock)
				},
			}
			return &DockerClient{
				logger:     logger,
				socketPath: sock,
				httpClient: &http.Client{Transport: transport},
			}
		}
	}
	return nil
}

func (d *DockerClient) Runtime() Runtime { return RuntimeDocker }

// ListNetworks returns all Docker-managed networks with their subnets.
func (d *DockerClient) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	body, err := d.get(ctx, "/networks")
	if err != nil {
		return nil, fmt.Errorf("docker list networks: %w", err)
	}
	defer body.Close()

	var raw []dockerNetwork
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("docker decode networks: %w", err)
	}

	var networks []NetworkInfo
	for _, n := range raw {
		info := NetworkInfo{
			ID:     n.ID,
			Name:   n.Name,
			Driver: n.Driver,
		}
		if n.IPAM.Config != nil {
			for _, cfg := range n.IPAM.Config {
				if cfg.Subnet != "" {
					info.Subnets = append(info.Subnets, cfg.Subnet)
				}
			}
		}
		networks = append(networks, info)
	}
	d.logger.Debug(fmt.Sprintf("Container runtime: Docker returned %d networks", len(networks)))
	return networks, nil
}

// ExcludedCIDRs returns CIDRs for all non-routable Docker networks (bridge, overlay, etc).
func (d *DockerClient) ExcludedCIDRs(ctx context.Context) ([]string, error) {
	networks, err := d.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}
	var cidrs []string
	for _, n := range networks {
		if IsExcludedDriver(n.Driver) {
			cidrs = append(cidrs, n.Subnets...)
			if len(n.Subnets) > 0 {
				d.logger.Debug(fmt.Sprintf("Container runtime: excluding Docker network %q (%s) subnets: %v", n.Name, n.Driver, n.Subnets))
			}
		}
	}
	return cidrs, nil
}

// ListContainers returns all running containers with network details.
func (d *DockerClient) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	body, err := d.get(ctx, "/containers/json")
	if err != nil {
		return nil, fmt.Errorf("docker list containers: %w", err)
	}
	defer body.Close()

	var raw []dockerContainer
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("docker decode containers: %w", err)
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
			for name, net := range c.NetworkSettings.Networks {
				info.Networks[name] = ContainerNetwork{
					NetworkID:   net.NetworkID,
					NetworkName: name,
					IPAddress:   net.IPAddress,
					MacAddress:  net.MacAddress,
					Gateway:     net.Gateway,
				}
			}
		}
		containers = append(containers, info)
	}
	d.logger.Debug(fmt.Sprintf("Container runtime: Docker returned %d running containers", len(containers)))
	return containers, nil
}

func (d *DockerClient) get(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}
	return resp.Body, nil
}

