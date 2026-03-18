package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// RemoteRequest describes how to fetch configuration from a remote URL.
type RemoteRequest struct {
	URL    string // Required: the endpoint to fetch from
	Method string // GET (default) or POST
	Header string // Optional header name (e.g. "Authorization")
	Token  string // Optional header value / bearer token
}

// IdentityPayload is the JSON body sent with POST requests so the remote
// endpoint can identify the calling node and return tailored configuration.
type IdentityPayload struct {
	Hostname string      `json:"hostname"`
	NodeID   string      `json:"nodeId"`
	OSInfo   OSInfo      `json:"osInfo"`
	IPInfo   IPInfo      `json:"ipInfo"`
}

// OSInfo contains operating system metadata.
type OSInfo struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

// IPInfo contains the node's network identity.
type IPInfo struct {
	PublicIPv4  string `json:"publicIPv4,omitempty"`
	PublicIPv6  string `json:"publicIPv6,omitempty"`
	RFC1918IPv4 string `json:"rfc1918IPv4,omitempty"`
	CGNATIPv4   string `json:"cgnatIPv4,omitempty"`
}

// LoadFromURL fetches JSON configuration from a remote endpoint, parses it,
// applies defaults, and validates it — same contract as Load but over HTTP.
func LoadFromURL(req RemoteRequest) (Config, error) {
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if method == http.MethodPost {
		payload := buildIdentityPayload()
		data, err := json.Marshal(payload)
		if err != nil {
			return Config{}, fmt.Errorf("marshal identity payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return Config{}, fmt.Errorf("create remote config request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.Header != "" && req.Token != "" {
		httpReq.Header.Set(req.Header, req.Token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return Config{}, fmt.Errorf("remote config fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Config{}, fmt.Errorf("remote config returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Config{}, fmt.Errorf("read remote config response: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode remote config: %w", err)
	}
	cfg.ApplyBuiltInDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("remote config validation: %w", err)
	}
	return cfg, nil
}

// buildIdentityPayload collects lightweight host metadata for the POST body.
// This runs before the full runtime context resolver, so it uses stdlib only.
func buildIdentityPayload() IdentityPayload {
	hostname, _ := os.Hostname()
	nodeID := readMachineIDForPayload()
	if nodeID == "" {
		nodeID = hostname
	}

	payload := IdentityPayload{
		Hostname: hostname,
		NodeID:   nodeID,
		OSInfo: OSInfo{
			OS:           runtime.GOOS,
			Architecture: runtime.GOARCH,
		},
	}

	// Collect first RFC1918 address from network interfaces
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil || ip.To4() == nil {
					continue
				}
				if isPrivateIPv4(ip) && payload.IPInfo.RFC1918IPv4 == "" {
					payload.IPInfo.RFC1918IPv4 = ip.String()
				}
			}
		}
	}

	return payload
}

// readMachineIDForPayload reads a stable machine UUID from the OS.
// Duplicates the logic from runtimectx to avoid a circular dependency.
func readMachineIDForPayload() string {
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
	}
	return ""
}

// isPrivateIPv4 returns true if ip is in an RFC 1918 range.
func isPrivateIPv4(ip net.IP) bool {
	private := []struct{ start, end net.IP }{
		{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")},
		{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")},
		{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")},
	}
	for _, r := range private {
		if bytes.Compare(ip.To4(), r.start.To4()) >= 0 && bytes.Compare(ip.To4(), r.end.To4()) <= 0 {
			return true
		}
	}
	return false
}

