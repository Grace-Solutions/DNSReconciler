package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	remoteMaxRetries    = 5
	remoteInitialDelay  = 1 * time.Second
	remoteMaxDelay      = 16 * time.Second
	remoteBackoffFactor = 2
)

// RemoteRequest describes how to fetch configuration from a remote URL.
type RemoteRequest struct {
	URL    string // Required: the endpoint to fetch from
	Method string // GET (default) or POST
	Header string // Optional header name (e.g. "Authorization")
	Token  string // Optional header value / bearer token
}

// RemoteCache holds cached response metadata for conditional re-fetches.
type RemoteCache struct {
	ETag         string    // ETag header from last successful response
	LastModified string    // Last-Modified header from last successful response
	FetchedAt    time.Time // When the config was last fetched
	Config       *Config   // Cached config (nil if never fetched)
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
	PublicIPv4         string              `json:"publicIPv4,omitempty"`
	PublicIPv6         string              `json:"publicIPv6,omitempty"`
	RFC1918IPv4        string              `json:"rfc1918IPv4,omitempty"`
	CGNATIPv4          string              `json:"cgnatIPv4,omitempty"`
	InterfaceAddresses map[string][]string `json:"interfaceAddresses,omitempty"`
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

	var raw []byte
	delay := remoteInitialDelay
	var lastErr error

	for attempt := 0; attempt <= remoteMaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[Information] - Remote config: retry %d/%d in %s", attempt, remoteMaxRetries, delay)
			time.Sleep(delay)
			delay *= remoteBackoffFactor
			if delay > remoteMaxDelay {
				delay = remoteMaxDelay
			}

			// Rebuild the request for retry (body may have been consumed)
			httpReq, err = http.NewRequest(method, req.URL, nil)
			if err != nil {
				return Config{}, fmt.Errorf("create remote config request: %w", err)
			}
			httpReq.Header.Set("Accept", "application/json")
			if method == http.MethodPost {
				data, _ := json.Marshal(buildIdentityPayload())
				httpReq.Body = io.NopCloser(bytes.NewReader(data))
				httpReq.Header.Set("Content-Type", "application/json")
			}
			if req.Header != "" && req.Token != "" {
				httpReq.Header.Set(req.Header, req.Token)
			}
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("remote config fetch failed: %w", err)
			log.Printf("[Warning] - Remote config: attempt %d failed: %s", attempt+1, lastErr)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			lastErr = fmt.Errorf("remote config returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
			log.Printf("[Warning] - Remote config: attempt %d returned HTTP %d", attempt+1, resp.StatusCode)
			continue
		}

		raw, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read remote config response: %w", err)
			log.Printf("[Warning] - Remote config: attempt %d read failed: %s", attempt+1, lastErr)
			continue
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return Config{}, fmt.Errorf("remote config failed after %d attempts: %w", remoteMaxRetries+1, lastErr)
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

// LoadFromURLCached fetches remote configuration only when the TTL has
// expired or the cache is empty. Uses ETag / If-Modified-Since headers to
// avoid re-parsing unchanged content. Returns the (possibly cached) config.
func LoadFromURLCached(req RemoteRequest, cache *RemoteCache, ttl time.Duration) (config Config, changed bool, err error) {
	if cache.Config != nil && time.Since(cache.FetchedAt) < ttl {
		return *cache.Config, false, nil
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if method == http.MethodPost {
		data, err := json.Marshal(buildIdentityPayload())
		if err != nil {
			return Config{}, false, fmt.Errorf("marshal identity payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return Config{}, false, fmt.Errorf("create remote config request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.Header != "" && req.Token != "" {
		httpReq.Header.Set(req.Header, req.Token)
	}

	// Conditional request headers
	if cache.ETag != "" {
		httpReq.Header.Set("If-None-Match", cache.ETag)
	}
	if cache.LastModified != "" {
		httpReq.Header.Set("If-Modified-Since", cache.LastModified)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		if cache.Config != nil {
			log.Printf("[Warning] - Remote config: fetch failed, using cached config: %s", err)
			cache.FetchedAt = time.Now() // reset TTL to avoid hammering
			return *cache.Config, false, nil
		}
		return Config{}, false, fmt.Errorf("remote config fetch failed: %w", err)
	}
	defer resp.Body.Close()

	// 304 Not Modified — content unchanged
	if resp.StatusCode == http.StatusNotModified && cache.Config != nil {
		cache.FetchedAt = time.Now()
		return *cache.Config, false, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if cache.Config != nil {
			log.Printf("[Warning] - Remote config: HTTP %d, using cached config: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
			cache.FetchedAt = time.Now()
			return *cache.Config, false, nil
		}
		return Config{}, false, fmt.Errorf("remote config returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Config{}, false, fmt.Errorf("read remote config response: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("decode remote config: %w", err)
	}
	cfg.ApplyBuiltInDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("remote config validation: %w", err)
	}

	// Update cache
	cache.ETag = resp.Header.Get("ETag")
	cache.LastModified = resp.Header.Get("Last-Modified")
	cache.FetchedAt = time.Now()
	cache.Config = &cfg

	return cfg, true, nil
}

// virtualBridgePrefixes are interface name prefixes for virtual/container bridges
// that should be excluded from identity payload address collection.
var virtualBridgePrefixes = []string{
	"docker", "br-", "veth", "virbr", "cni", "flannel", "calico", "weave",
}

// isVirtualBridge returns true if the interface name matches a known virtual bridge prefix.
func isVirtualBridge(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range virtualBridgePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
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
		IPInfo: IPInfo{
			InterfaceAddresses: make(map[string][]string),
		},
	}

	// Collect addresses from all non-loopback, non-virtual interfaces
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if isVirtualBridge(iface.Name) {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					continue
				}

				// Collect interface addresses (both v4 and v6)
				payload.IPInfo.InterfaceAddresses[iface.Name] = append(
					payload.IPInfo.InterfaceAddresses[iface.Name], ip.String())

				if ip.To4() == nil {
					continue // IPv4-only classification below
				}

				if isPrivateIPv4(ip) && payload.IPInfo.RFC1918IPv4 == "" {
					payload.IPInfo.RFC1918IPv4 = ip.String()
				}
				if isCGNATIPv4(ip) && payload.IPInfo.CGNATIPv4 == "" {
					payload.IPInfo.CGNATIPv4 = ip.String()
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

// isCGNATIPv4 returns true if ip is in the CGNAT range (100.64.0.0/10).
func isCGNATIPv4(ip net.IP) bool {
	start := net.ParseIP("100.64.0.0").To4()
	end := net.ParseIP("100.127.255.255").To4()
	return bytes.Compare(ip.To4(), start) >= 0 && bytes.Compare(ip.To4(), end) <= 0
}

