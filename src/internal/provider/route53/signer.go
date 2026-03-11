package route53

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	awsService   = "route53"
	awsAlgorithm = "AWS4-HMAC-SHA256"
)

// awsSigner signs HTTP requests using AWS Signature Version 4.
type awsSigner struct {
	accessKeyID     string
	secretAccessKey string
	region          string
}

// sign adds AWS Signature V4 headers to the request.
func (s *awsSigner) sign(req *http.Request, body []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("host", req.Host)

	// 1. Canonical request
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQueryString := req.URL.Query().Encode()

	signedHeaderKeys := []string{}
	headerMap := map[string]string{}
	for key := range req.Header {
		lower := strings.ToLower(key)
		signedHeaderKeys = append(signedHeaderKeys, lower)
		headerMap[lower] = strings.TrimSpace(req.Header.Get(key))
	}
	sort.Strings(signedHeaderKeys)

	var canonicalHeaders strings.Builder
	for _, k := range signedHeaderKeys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(headerMap[k])
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(signedHeaderKeys, ";")

	payloadHash := sha256Hex(body)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	// 2. String to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, s.region, awsService)
	stringToSign := strings.Join([]string{
		awsAlgorithm,
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 3. Signing key
	signingKey := deriveSigningKey(s.secretAccessKey, dateStamp, s.region, awsService)

	// 4. Signature
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))

	// 5. Authorization header
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		awsAlgorithm, s.accessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:])
}

// readBody reads and returns the request body bytes, then resets the body.
func readBody(req *http.Request) []byte {
	if req.Body == nil {
		return []byte{}
	}
	data, _ := io.ReadAll(req.Body)
	return data
}

