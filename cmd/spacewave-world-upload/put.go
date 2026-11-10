package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// The SigV4 constants, as db/block/store/s3/sign.go has them.
const (
	signAlgorithm   = "AWS4-HMAC-SHA256"
	signService     = "s3"
	signRequestName = "aws4_request"
	amzDateFormat   = "20060102T150405Z"
	dateStampFormat = "20060102"
)

// putTimeout bounds one PUT, a pack of up to 128 MiB included.
const putTimeout = 30 * time.Minute

// putter sends signed path-style PUT requests to one bucket. It has no other
// method and never retries.
type putter struct {
	http     *http.Client
	endpoint *url.URL
	region   string
	bucket   string
	creds    *creds
	now      func() time.Time
	// ifNoneMatch sends a signed If-None-Match: * on every PUT, so the
	// service refuses with 412 to overwrite a key that exists.
	ifNoneMatch bool
	// checksumHeader sends a signed x-amz-checksum-sha256 on every PUT and
	// checks the echoed one.
	checksumHeader bool
}

// object is one body to PUT with its digests.
type object struct {
	key    string
	size   int64
	data   []byte
	sha256 [sha256.Size]byte
	md5    []byte
}

// statusBadSum is the manifest status of a 200 whose echoed sha256 is not
// the one sent. readManifest does not count it as done.
const statusBadSum = "200-badsum"

// put PUTs the object and returns the status to record: the response code,
// statusBadSum for a 200 whose echoed checksum is not ours, or "" when no
// response came. Any status but "200" comes with an error. echo is the
// x-amz-checksum-sha256 the 200 echoed, "" for none.
func (p *putter) put(ctx context.Context, obj *object) (status string, echo string, err error) {
	// Build the request on the path-style object URL.
	u := *p.endpoint
	u.Path = "/" + p.bucket + "/" + obj.key
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	ctx, cancel := context.WithTimeout(ctx, putTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(obj.data))
	if err != nil {
		return "", "", err
	}
	req.ContentLength = int64(len(obj.data))

	// Carry both checksums, so the service refuses a body that differs.
	sumB64 := base64.StdEncoding.EncodeToString(obj.sha256[:])
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(obj.md5))
	if p.checksumHeader {
		req.Header.Set("X-Amz-Checksum-Sha256", sumB64)
	}
	if p.ifNoneMatch {
		req.Header.Set("If-None-Match", "*")
	}
	p.sign(req, hex.EncodeToString(obj.sha256[:]), p.now())

	// Send it once.
	resp, err := p.http.Do(req)
	if err != nil {
		return "", "", errors.Wrapf(err, "PUT %s", obj.key)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// Refuse any status but 200, naming a key the bucket already holds.
	status = strconv.Itoa(resp.StatusCode)
	if resp.StatusCode == http.StatusPreconditionFailed && p.ifNoneMatch {
		return status, "", errors.Errorf("PUT %s: status 412: %s exists in the bucket; not overwritten", obj.key, obj.key)
	}
	if resp.StatusCode != http.StatusOK {
		return status, "", errors.Errorf("PUT %s: status %d: %s", obj.key, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Refuse a 200 whose echoed checksum is not the one sent.
	echo = resp.Header.Get("X-Amz-Checksum-Sha256")
	if p.checksumHeader && echo != "" && echo != sumB64 {
		return statusBadSum, echo, errors.Errorf("PUT %s: status 200 but the service reports sha256 %s, sent %s", obj.key, echo, sumB64)
	}
	return status, echo, nil
}

// sign signs req with AWS Signature Version 4 over every header it carries,
// as db/block/store/s3/sign.go does. payloadHash is the hex sha256 of the
// body.
func (p *putter) sign(req *http.Request, payloadHash string, now time.Time) {
	// Set the date and payload hash the signature covers.
	t := now.UTC()
	req.Header.Set("X-Amz-Date", t.Format(amzDateFormat))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Canonicalize the host and headers.
	names := []string{"host"}
	values := map[string]string{"host": req.URL.Host}
	for name, vals := range req.Header {
		l := strings.ToLower(name)
		if l == "authorization" {
			continue
		}
		names = append(names, l)
		values[l] = strings.TrimSpace(strings.Join(vals, ","))
	}
	slices.Sort(names)
	var canon strings.Builder
	for _, n := range names {
		canon.WriteString(n + ":" + values[n] + "\n")
	}
	signed := strings.Join(names, ";")

	// Sign the canonical request with the day's derived key.
	scope := t.Format(dateStampFormat) + "/" + p.region + "/" + signService + "/" + signRequestName
	canonical := strings.Join([]string{req.Method, uriEncode(req.URL.Path), req.URL.RawQuery, canon.String(), signed, payloadHash}, "\n")
	toSign := strings.Join([]string{signAlgorithm, t.Format(amzDateFormat), scope, hexSHA256(canonical)}, "\n")
	k := hmacSHA256([]byte("AWS4"+p.creds.secretAccessKey), t.Format(dateStampFormat))
	k = hmacSHA256(k, p.region)
	k = hmacSHA256(k, signService)
	k = hmacSHA256(k, signRequestName)
	req.Header.Set("Authorization", signAlgorithm+" Credential="+p.creds.accessKeyID+"/"+scope+
		", SignedHeaders="+signed+", Signature="+hex.EncodeToString(hmacSHA256(k, toSign)))
}

// uriEncode percent-encodes a path per SigV4, leaving '/' as is.
func uriEncode(s string) string {
	const hexChars = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9',
			ch == '-', ch == '_', ch == '.', ch == '~', ch == '/':
			b.WriteByte(ch)
		default:
			b.WriteByte('%')
			b.WriteByte(hexChars[ch>>4])
			b.WriteByte(hexChars[ch&0xF])
		}
	}
	return b.String()
}

// hmacSHA256 returns the HMAC-SHA256 of data under key.
func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// hexSHA256 returns the hex sha256 of s.
func hexSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
