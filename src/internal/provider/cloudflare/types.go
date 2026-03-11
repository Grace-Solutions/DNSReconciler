package cloudflare

// cfDNSRecord represents a Cloudflare DNS record in API responses/requests.
type cfDNSRecord struct {
	ID      string  `json:"id,omitempty"`
	Type    string  `json:"type"`
	Name    string  `json:"name"`
	Content string  `json:"content"`
	TTL     int     `json:"ttl"`
	Proxied bool    `json:"proxied"`
	Comment string  `json:"comment,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// cfListResponse wraps the Cloudflare list DNS records response.
type cfListResponse struct {
	Success  bool          `json:"success"`
	Errors   []cfError     `json:"errors"`
	Result   []cfDNSRecord `json:"result"`
	Messages []cfMessage   `json:"messages"`
}

// cfSingleResponse wraps a single-record response.
type cfSingleResponse struct {
	Success bool        `json:"success"`
	Errors  []cfError   `json:"errors"`
	Result  cfDNSRecord `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

