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
	"strconv"
	"strings"
	"time"
)

const (
	signAlgorithm   = "AWS4-HMAC-SHA256"
	signService     = "s3"
	signRequestName = "aws4_request"
	amzDateFormat   = "20060102T150405Z"
	dateStampFormat = "20060102"
)

// unsignedPayload is the payload hash of a presigned request, whose body the
// signature does not cover.
const unsignedPayload = "UNSIGNED-PAYLOAD"

// signV4 signs an http request using AWS Signature Version 4.
// payloadHash must be hex-encoded sha256 of the request body.
// If the client has no access key, only the date and content-sha256 headers
// are added (anonymous request).
func (c *Client) signV4(req *http.Request, payloadHash string, now time.Time) {
	// Set the headers every request carries, signed or anonymous.
	t := now.UTC()
	req.Header.Set("X-Amz-Date", t.Format(amzDateFormat))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if c.token != "" {
		req.Header.Set("X-Amz-Security-Token", c.token)
	}
	if c.accessKey == "" {
		return
	}

	// Sign the canonical request and attach the authorization header.
	signedHeaders, canonicalHeaders := canonicalRequestHeaders(req.URL.Host, req.Header)
	credScope, signature := c.sign(t, strings.Join([]string{
		req.Method,
		uriEncode(req.URL.Path, false),
		canonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n"))
	req.Header.Set("Authorization",
		signAlgorithm+" Credential="+c.accessKey+"/"+credScope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature)
}

// presignV4 adds an AWS Signature Version 4 query signature to u that
// authorizes method on u with header for expires from now. The request must
// send each header with the given value. The signature does not cover the
// body.
func (c *Client) presignV4(method string, u *url.URL, header http.Header, expires time.Duration, now time.Time) {
	// Canonicalize the headers the upload must send.
	t := now.UTC()
	signedHeaders, canonicalHeaders := canonicalRequestHeaders(u.Host, header)

	// Add the signing parameters the signature covers to the query.
	query := u.Query()
	query.Set("X-Amz-Algorithm", signAlgorithm)
	query.Set("X-Amz-Credential", c.accessKey+"/"+c.credentialScope(t))
	query.Set("X-Amz-Date", t.Format(amzDateFormat))
	query.Set("X-Amz-Expires", strconv.Itoa(int(expires/time.Second)))
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	if c.token != "" {
		query.Set("X-Amz-Security-Token", c.token)
	}

	// Sign the canonical request and append the signature.
	_, signature := c.sign(t, strings.Join([]string{
		method,
		uriEncode(u.Path, false),
		canonicalQuery(query),
		canonicalHeaders,
		signedHeaders,
		unsignedPayload,
	}, "\n"))
	query.Set("X-Amz-Signature", signature)
	u.RawQuery = canonicalQuery(query)
}

// sign returns the credential scope at t and the hex signature of the
// canonical request.
func (c *Client) sign(t time.Time, canonicalRequest string) (credScope, signature string) {
	// Build the string to sign over the canonical request.
	credScope = c.credentialScope(t)
	stringToSign := strings.Join([]string{
		signAlgorithm,
		t.Format(amzDateFormat),
		credScope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	// Derive the signing key for the day and region, then sign.
	dateStamp := t.Format(dateStampFormat)
	kDate := hmacSHA256([]byte("AWS4"+c.secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, c.region)
	kService := hmacSHA256(kRegion, signService)
	kSigning := hmacSHA256(kService, signRequestName)
	return credScope, hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
}

// credentialScope returns the credential scope of a signature at t.
func (c *Client) credentialScope(t time.Time) string {
	return t.Format(dateStampFormat) + "/" + c.region + "/" + signService + "/" + signRequestName
}

// canonicalRequestHeaders returns the SignedHeaders list and CanonicalHeaders
// block of host and header. CanonicalHeaders ends with a trailing newline as
// required by SigV4.
func canonicalRequestHeaders(host string, header http.Header) (signed string, canonical string) {
	// Collect the lowercase names and trimmed values of host and header.
	headers := map[string]string{
		"host": host,
	}
	keys := []string{"host"}
	for name, vals := range header {
		lname := strings.ToLower(name)
		if lname == "authorization" {
			continue
		}
		keys = append(keys, lname)
		headers[lname] = strings.TrimSpace(strings.Join(vals, ","))
	}

	// Write the sorted names and their values.
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
