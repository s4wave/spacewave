//go:build !tinygo

package block_store_s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	signAlgorithm   = "AWS4-HMAC-SHA256"
	signService     = "s3"
	signRequestName = "aws4_request"
)

// signV4 signs an http request using AWS Signature Version 4.
// payloadHash must be hex-encoded sha256 of the request body.
// If the client has no access key, only the date and content-sha256 headers
// are added (anonymous request).
func (c *Client) signV4(req *http.Request, payloadHash string, now time.Time) {
	// Format the signing time for the request date and credential scope.
	t := now.UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	// Attach payload and date headers, including the session token when present.
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if c.token != "" {
		req.Header.Set("X-Amz-Security-Token", c.token)
	}
	if c.accessKey == "" {
		return
	}

	// Canonicalize the request headers and object path for signing.
	signedHeaders, canonicalHeaders := canonicalRequestHeaders(req)
	canonicalURI := uriEncode(req.URL.Path, false)

	// Build the canonical S3 request from its method, path, query, and payload.
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// Build the string to sign for the S3 region and request date.
	credScope := dateStamp + "/" + c.region + "/" + signService + "/" + signRequestName
	stringToSign := strings.Join([]string{
		signAlgorithm,
		amzDate,
		credScope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	// Derive the S3 signing key and calculate the request signature.
	kDate := hmacSHA256([]byte("AWS4"+c.secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, c.region)
	kService := hmacSHA256(kRegion, signService)
	kSigning := hmacSHA256(kService, signRequestName)
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	// Attach the credential scope, signed headers, and signature to the request.
	req.Header.Set("Authorization",
		signAlgorithm+" Credential="+c.accessKey+"/"+credScope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature)
}

// canonicalRequestHeaders returns the SignedHeaders list and CanonicalHeaders block.
// CanonicalHeaders ends with a trailing newline as required by SigV4.
func canonicalRequestHeaders(req *http.Request) (signed string, canonical string) {
	// Collect normalized request headers while excluding authorization.
	headers := map[string]string{
		"host": req.URL.Host,
	}
	keys := []string{"host"}
	for name, vals := range req.Header {
		lname := strings.ToLower(name)
		if lname == "authorization" {
			continue
		}
		keys = append(keys, lname)
		headers[lname] = strings.TrimSpace(strings.Join(vals, ","))
	}

	// Write sorted canonical headers and the signed header list.
	slices.Sort(keys)
	var hb, sb strings.Builder
	for i, k := range keys {
		hb.WriteString(k)
		hb.WriteByte(':')
		hb.WriteString(headers[k])
		hb.WriteByte('\n')
		if i > 0 {
			sb.WriteByte(';')
		}
		sb.WriteString(k)
	}
	return sb.String(), hb.String()
}

// canonicalQuery encodes query with sorted keys and values, percent-encoded
// per SigV4. The request sends the same string, so the server's canonical
// form matches the signed one.
func canonicalQuery(query url.Values) string {
	keys := slices.Sorted(maps.Keys(query))
	var b strings.Builder
	for _, k := range keys {
		values := slices.Clone(query[k])
		slices.Sort(values)
		for _, v := range values {
			if b.Len() != 0 {
				b.WriteByte('&')
			}
			b.WriteString(uriEncode(k, true))
			b.WriteByte('=')
			b.WriteString(uriEncode(v, true))
		}
	}
	return b.String()
}

// uriEncode percent-encodes per AWS S3 SigV4 rules.
// If encodeSlash is false, '/' is left unescaped (used for the canonical URI).
func uriEncode(s string, encodeSlash bool) string {
	// Encode the object path or query value using S3 percent-encoding rules.
	const hexChars = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9'),
			ch == '-', ch == '_', ch == '.', ch == '~':
			b.WriteByte(ch)
		case ch == '/' && !encodeSlash:
			b.WriteByte(ch)
		default:
			b.WriteByte('%')
			b.WriteByte(hexChars[ch>>4])
			b.WriteByte(hexChars[ch&0xF])
		}
	}
	return b.String()
}

// hmacSHA256 returns HMAC-SHA256 of data using key.
func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// hexSHA256 returns the hex-encoded sha256 of data.
func hexSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
