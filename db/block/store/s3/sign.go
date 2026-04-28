//go:build !tinygo

package block_store_s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
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
	t := now.UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if c.token != "" {
		req.Header.Set("X-Amz-Security-Token", c.token)
	}
	if c.accessKey == "" {
		return
	}

	signedHeaders, canonicalHeaders := canonicalRequestHeaders(req)
	canonicalQuery := req.URL.Query().Encode()
	canonicalURI := uriEncode(req.URL.Path, false)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credScope := dateStamp + "/" + c.region + "/" + signService + "/" + signRequestName
	stringToSign := strings.Join([]string{
		signAlgorithm,
		amzDate,
		credScope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+c.secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, c.region)
	kService := hmacSHA256(kRegion, signService)
	kSigning := hmacSHA256(kService, signRequestName)
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization",
		signAlgorithm+" Credential="+c.accessKey+"/"+credScope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature)
}

// canonicalRequestHeaders returns the SignedHeaders list and CanonicalHeaders block.
// CanonicalHeaders ends with a trailing newline as required by SigV4.
func canonicalRequestHeaders(req *http.Request) (signed string, canonical string) {
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
	sort.Strings(keys)
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
