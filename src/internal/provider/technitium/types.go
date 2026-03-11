package technitium

// techResponse wraps common Technitium API response fields.
type techResponse struct {
	Status   string `json:"status"`
	ErrorMsg string `json:"errorMessage,omitempty"`
}

// techListResponse wraps the zones/records/get response.
type techListResponse struct {
	Status   string         `json:"status"`
	ErrorMsg string         `json:"errorMessage,omitempty"`
	Response techRecordList `json:"response"`
}

type techRecordList struct {
	Records []techRecord `json:"records"`
}

// techRecord represents a single DNS record from the Technitium API.
type techRecord struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	TTL      int           `json:"ttl"`
	Disabled bool          `json:"disabled"`
	RData    techRData     `json:"rData"`
	Comments string        `json:"comments,omitempty"`
}

// techRData holds type-specific record data.
type techRData struct {
	IPAddress   string `json:"ipAddress,omitempty"`   // A records
	IPv6Address string `json:"ipv6Address,omitempty"` // AAAA records
	CName       string `json:"cname,omitempty"`       // CNAME records
	Text        string `json:"text,omitempty"`         // TXT records
}

