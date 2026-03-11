package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// APIClient wraps an http.Client with common JSON/REST patterns.
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Headers    map[string]string
	Logger     *logging.Logger
}

// NewAPIClient creates a client with reasonable defaults.
func NewAPIClient(baseURL string, headers map[string]string, logger *logging.Logger) *APIClient {
	return &APIClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Headers: headers,
		Logger:  logger,
	}
}

// Do executes a request and decodes the response JSON into dest.
// If dest is nil the body is discarded.
func (c *APIClient) Do(ctx context.Context, method, path string, body any, dest any) (*http.Response, error) {
	var bodyReader io.Reader
	var bodySnapshot string
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodySnapshot = string(encoded)
		bodyReader = bytes.NewReader(encoded)
	}

	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request %s %s: %w", method, url, err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	if bodySnapshot != "" {
		c.Logger.Debug(fmt.Sprintf("HTTP request: %s %s body=%s", method, url, bodySnapshot))
	} else {
		c.Logger.Debug(fmt.Sprintf("HTTP request: %s %s", method, url))
	}
	start := time.Now()

	resp, err := c.HTTPClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		c.Logger.Warning(fmt.Sprintf("HTTP request failed: %s %s (%s): %s", method, url, elapsed, err))
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, fmt.Errorf("read response body %s %s: %w", method, url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.Logger.Warning(fmt.Sprintf("HTTP response: %s %s → %d (%s)", method, url, resp.StatusCode, elapsed))
		return resp, &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			URL:        url,
			Body:       string(respBody),
		}
	}

	c.Logger.Debug(fmt.Sprintf("HTTP response: %s %s → %d (%s)", method, url, resp.StatusCode, elapsed))

	if dest != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return resp, fmt.Errorf("decode response %s %s: %w", method, url, err)
		}
	}

	return resp, nil
}

// APIError represents a non-2xx response.
type APIError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *APIError) Error() string {
	snippet := e.Body
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}
	return fmt.Sprintf("API %s %s returned %d: %s", e.Method, e.URL, e.StatusCode, snippet)
}

