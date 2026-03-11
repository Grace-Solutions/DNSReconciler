package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenCache caches an OAuth2 access token and handles refresh.
type tokenCache struct {
	mu           sync.Mutex
	httpClient   *http.Client
	tenantID     string
	clientID     string
	clientSecret string
	token        string
	expiry       time.Time
}

// newTokenCache creates a new token cache for client credentials flow.
func newTokenCache(httpClient *http.Client, tenantID, clientID, clientSecret string) *tokenCache {
	return &tokenCache{
		httpClient:   httpClient,
		tenantID:     tenantID,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// getToken returns a valid access token, refreshing if needed.
func (tc *tokenCache) getToken(ctx context.Context) (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Return cached token if still valid (with 60s buffer)
	if tc.token != "" && time.Now().Add(60*time.Second).Before(tc.expiry) {
		return tc.token, nil
	}

	// Acquire new token via client credentials grant
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tc.tenantID)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", tc.clientID)
	form.Set("client_secret", tc.clientSecret)
	form.Set("scope", "https://management.azure.com/.default")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("azure auth: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure auth: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("azure auth: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return "", fmt.Errorf("azure auth: token endpoint returned %d: %s", resp.StatusCode, snippet)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("azure auth: decode token response: %w", err)
	}

	tc.token = tokenResp.AccessToken
	// Parse expiry — Azure returns seconds as a string
	var expiresIn int
	fmt.Sscanf(tokenResp.ExpiresIn, "%d", &expiresIn)
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	tc.expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)

	return tc.token, nil
}

