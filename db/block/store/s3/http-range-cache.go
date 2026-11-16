//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// HTTPRangeCache reads bounded ranges through an optional private HTTP cache.
// The cache must preserve the origin's bytes and validate object identity.
// A failed attempt never changes the context or transport of the direct read.
type HTTPRangeCache struct {
	// endpoint is the private Worker's origin.
	endpoint string
	// token authenticates requests, only in the Authorization header.
	token string
	// client bounds the complete attempt, including the response body.
	client *http.Client
	// counts records completed reads by source.
	counts [3]atomic.Uint64
	// bytes records completed bytes by source.
	bytes [3]atomic.Uint64
}

// NewHTTPRangeCache builds an optional cache with a 750 ms total deadline.
// An incomplete or invalid setting disables it. HTTP is allowed only on literal
// loopback addresses for local runtimes. Redirects are never followed:
// the private token must reach only the configured endpoint.
func NewHTTPRangeCache(endpoint, token string) *HTTPRangeCache {
	// Accept only a private origin without URL credentials or query parameters.
	u, err := url.Parse(endpoint)
	if err != nil || token == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil
	}
	if u.Scheme != "https" && (u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return nil
	}

	// Reuse connections, but never follow a redirect with the token.
	return &HTTPRangeCache{endpoint: strings.TrimSuffix(endpoint, "/"), token: token, client: &http.Client{
		Transport: newHTTPClient().Transport, Timeout: 750 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Counts returns a concurrent snapshot; each counter grows independently.
func (c *HTTPRangeCache) Counts() HTTPRangeCacheCounts {
	// Copy the counters without holding up a request.
	var out HTTPRangeCacheCounts
	for i := range out.Requests {
		out.Requests[i] = c.counts[i].Load()
		out.Bytes[i] = c.bytes[i].Load()
	}
	return out
}

// Read reads one range, falling back to lower on every cache failure.
// The 1 MiB bound matches the Worker; larger reads keep today's direct path.
func (c *HTTPRangeCache) Read(ctx context.Context, lower ObjectStore, bucket, key string, off int64, length int) ([]byte, error) {
	// Keep disabled, empty and large reads on their original path.
	if c == nil || off < 0 || length <= 0 || length > 1<<20 {
		return lower.GetObjectRange(ctx, bucket, key, off, length)
	}

	// Attempt the optional cache once, without the S3 client's retry loop.
	if data, source, ok := c.read(ctx, bucket, key, off, length); ok {
		c.counts[source].Add(1)
		c.bytes[source].Add(uint64(len(data)))
		return data, nil
	}

	// Fall back with the caller's original context, even after a cache timeout.
	c.counts[2].Add(1)
	data, err := lower.GetObjectRange(ctx, bucket, key, off, length)
	c.bytes[2].Add(uint64(len(data)))
	return data, err
}

// read accepts only a complete, correctly framed range from the Worker.
func (c *HTTPRangeCache) read(ctx context.Context, bucket, key string, off int64, length int) ([]byte, int, bool) {
	// Escape the bucket and key as path components, keeping the token out of URLs.
	u := c.endpoint + "/" + url.PathEscape(bucket) + "/" + strings.ReplaceAll(url.PathEscape(key), "%2F", "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, false
	}
	end := off + int64(length) - 1
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(end, 10))
	req.Header.Set("Accept-Encoding", "identity")

	// Bound reading through EOF so a failed body can still fall back atomically.
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Encoding") != "" {
		return nil, 0, false
	}
	expected := "bytes " + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(end, 10) + "/"
	if !strings.HasPrefix(resp.Header.Get("Content-Range"), expected) {
		return nil, 0, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(length)+1))
	if err != nil || len(data) != length {
		return nil, 0, false
	}

	// Separate R2 hits from misses served by the origin through the Worker.
	switch resp.Header.Get("X-Cache") {
	case "HIT":
		return data, 0, true
	case "MISS":
		return data, 1, true
	default:
		return nil, 0, false
	}
}
