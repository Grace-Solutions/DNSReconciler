package powerdns

// pdnsZoneResponse represents a PowerDNS zone with its RRsets.
type pdnsZoneResponse struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	RRsets []pdnsRRset `json:"rrsets"`
}

// pdnsRRset represents a set of DNS records sharing a name and type.
type pdnsRRset struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	TTL        int             `json:"ttl"`
	Changetype string          `json:"changetype,omitempty"`
	Records    []pdnsRecord    `json:"records"`
	Comments   []pdnsComment   `json:"comments,omitempty"`
}

// pdnsRecord represents a single record within an RRset.
type pdnsRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

// pdnsComment represents a comment on an RRset.
type pdnsComment struct {
	Content    string `json:"content"`
	Account    string `json:"account,omitempty"`
	ModifiedAt int64  `json:"modified_at,omitempty"`
}

// pdnsPatchBody is the PATCH request body for zone modifications.
type pdnsPatchBody struct {
	RRsets []pdnsRRset `json:"rrsets"`
}

