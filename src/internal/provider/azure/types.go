package azure

// tokenResponse represents the Azure AD OAuth2 token response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// recordSetListResult represents the Azure DNS list response.
type recordSetListResult struct {
	Value    []recordSet `json:"value"`
	NextLink string      `json:"nextLink,omitempty"`
}

// recordSet represents an Azure DNS record set.
type recordSet struct {
	ID         string              `json:"id,omitempty"`
	Name       string              `json:"name"`
	Type       string              `json:"type,omitempty"`
	Etag       string              `json:"etag,omitempty"`
	Properties recordSetProperties `json:"properties"`
}

// recordSetProperties holds the DNS record data.
type recordSetProperties struct {
	TTL         int               `json:"TTL"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ARecords    []aRecord         `json:"ARecords,omitempty"`
	AAAARecords []aaaaRecord      `json:"AAAARecords,omitempty"`
	CNAMERecord *cnameRecord      `json:"CNAMERecord,omitempty"`
	TXTRecords  []txtRecord       `json:"TXTRecords,omitempty"`
}

type aRecord struct {
	IPv4Address string `json:"ipv4Address"`
}

type aaaaRecord struct {
	IPv6Address string `json:"ipv6Address"`
}

type cnameRecord struct {
	CNAME string `json:"cname"`
}

type txtRecord struct {
	Value []string `json:"value"`
}

// azureErrorResponse represents an Azure API error.
type azureErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

