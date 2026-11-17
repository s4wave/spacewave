//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Breaker settings. A Worker that keeps failing costs each read up to the
// cache timeout before the direct read starts; the breaker sends reads direct
// for a cool-off instead, then lets one probe find out whether it recovered.
const (
	// cacheTimeout bounds one complete cache attempt, body included.
	cacheTimeout = 750 * time.Millisecond
	// cacheDeadlineMargin is the time a direct read keeps after a failed
	// attempt; a caller with less than the timeout plus this goes direct.
	cacheDeadlineMargin = 250 * time.Millisecond
	// breakerConsecutive consecutive fallbacks open the breaker.
	breakerConsecutive = 5
	// breakerWindow is the span over which the fallback share is measured.
	breakerWindow = 10 * time.Second
	// breakerWindowMin is the fewest attempts in a window that can open it.
	breakerWindowMin = 20
	// breakerCoolOff is how long an open breaker sends reads direct.
	breakerCoolOff = 30 * time.Second
	// breakerQuotaCoolOff is the cool-off after a 429: the Worker's quota or
	// rate limit is spent, so one answer opens the breaker for longer.
	breakerQuotaCoolOff = 5 * time.Minute
)

// Result indexes of HTTPRangeCacheCounts.
const (
	// cacheHit is a read served from the Worker's copy.
	cacheHit = iota
	// cacheMiss is a read the Worker served from the origin.
	cacheMiss
	// cacheFallback is a failed attempt followed by a direct read.
	cacheFallback
	// cacheSkipped is a direct read made while the breaker was open.
	cacheSkipped
	// cacheDeadline is a direct read made because the caller's deadline was
	// too near for an attempt and a fallback.
	cacheDeadline
	// cacheRejected is a direct read made because bytes the Worker served
	// earlier in the same block read failed the block's hash.
	cacheRejected
	// cacheResults is the number of results.
	cacheResults
)

// HTTPRangeCacheBreaker is the state of the cache's breaker.
type HTTPRangeCacheBreaker int32

const (
	// BreakerClosed tries the cache on every eligible read.
	BreakerClosed HTTPRangeCacheBreaker = iota
	// BreakerOpen reads direct until the cool-off ends.
	BreakerOpen
	// BreakerHalfOpen lets one probe try the cache; other reads go direct.
	BreakerHalfOpen
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
	// now reads the clock; tests replace it.
	now func() time.Time
	// counts records completed reads by result.
	counts [cacheResults]atomic.Uint64
	// bytes records completed bytes by result.
	bytes [cacheResults]atomic.Uint64
	// breaker decides whether a read tries the cache.
	breaker cacheBreaker
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
	return &HTTPRangeCache{endpoint: strings.TrimSuffix(endpoint, "/"), token: token, now: time.Now, client: &http.Client{
		Transport: newHTTPClient().Transport, Timeout: cacheTimeout,
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
	out.Breaker = HTTPRangeCacheBreaker(c.breaker.shown.Load())
	return out
}

// Read reads one range, falling back to lower on every cache failure.
// The 1 MiB bound matches the Worker; larger reads keep today's direct path.
func (c *HTTPRangeCache) Read(ctx context.Context, lower ObjectStore, bucket, key string, off int64, length int) ([]byte, error) {
	// Keep disabled, empty and large reads on their original path.
	if c == nil || off < 0 || length <= 0 || length > 1<<20 {
		return lower.GetObjectRange(ctx, bucket, key, off, length)
	}

	// Go direct for the rest of a block read whose Worker bytes failed its hash.
	blockRead, _ := ctx.Value(rangeReadKey{}).(*rangeRead)
	if blockRead != nil && blockRead.direct.Load() {
		return c.direct(ctx, lower, cacheRejected, bucket, key, off, length)
	}

	// Go direct when a failed attempt would leave the direct read too little time.
	now := c.now()
	if deadline, ok := ctx.Deadline(); ok && deadline.Sub(now) < c.client.Timeout+cacheDeadlineMargin {
		return c.direct(ctx, lower, cacheDeadline, bucket, key, off, length)
	}

	// Go direct, with no added wait, while the breaker is open.
	attempt, probe := c.breaker.admit(now)
	if !attempt {
		return c.direct(ctx, lower, cacheSkipped, bucket, key, off, length)
	}

	// Attempt the optional cache once, without the S3 client's retry loop.
	data, source, ok, quota := c.read(ctx, bucket, key, off, length)
	c.breaker.record(c.now(), probe, ok, quota, !ok && ctx.Err() != nil)
	if ok {
		if blockRead != nil {
			blockRead.cached.Store(true)
		}
		c.counts[source].Add(1)
		c.bytes[source].Add(uint64(len(data)))
		return data, nil
	}

	// Fall back with the caller's original context, even after a cache timeout.
	return c.direct(ctx, lower, cacheFallback, bucket, key, off, length)
}

// rangeReadKey keys a block read's rangeRead in its context.
type rangeReadKey struct{}

// rangeRead follows the range reads of one block read. The block's hash is
// checked only after Read returns the Worker's bytes, so a read that fails the
// check sets direct and reads again: the Worker is not asked for the same
// bytes twice in one read.
type rangeRead struct {
	// cached is set when the Worker served bytes to this read.
	cached atomic.Bool
	// direct sends the read's remaining range reads to the bucket.
	direct atomic.Bool
}

// withRangeRead returns ctx carrying a new rangeRead for one block read.
func withRangeRead(ctx context.Context) (context.Context, *rangeRead) {
	read := &rangeRead{}
	return context.WithValue(ctx, rangeReadKey{}, read), read
}

// reject reports whether the read should be tried again from the bucket: the
// Worker served it bytes and it was not already sent direct. It sends the
// rest of the read direct.
func (r *rangeRead) reject() bool {
	return r.cached.Load() && r.direct.CompareAndSwap(false, true)
}

// direct reads from lower and counts the read under result.
func (c *HTTPRangeCache) direct(ctx context.Context, lower ObjectStore, result int, bucket, key string, off int64, length int) ([]byte, error) {
	c.counts[result].Add(1)
	data, err := lower.GetObjectRange(ctx, bucket, key, off, length)
	c.bytes[result].Add(uint64(len(data)))
	return data, err
}

// read accepts only a complete, correctly framed range from the Worker. It
// reports quota when the Worker answered 429.
func (c *HTTPRangeCache) read(ctx context.Context, bucket, key string, off int64, length int) (data []byte, source int, ok, quota bool) {
	// Escape the bucket and key as path components, keeping the token out of URLs.
	u := c.endpoint + "/" + url.PathEscape(bucket) + "/" + strings.ReplaceAll(url.PathEscape(key), "%2F", "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, false, false
	}
	end := off + int64(length) - 1
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(end, 10))
	req.Header.Set("Accept-Encoding", "identity")

	// Bound reading through EOF so a failed body can still fall back atomically.
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, 0, false, true
	}
	if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Encoding") != "" {
		return nil, 0, false, false
	}

	// Require the exact range, a numeric total past its end and an exact length.
	if !rangeFramed(resp.Header.Get("Content-Range"), off, end) || resp.ContentLength != int64(length) {
		return nil, 0, false, false
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, int64(length)+1))
	if err != nil || len(data) != length {
		return nil, 0, false, false
	}

	// Separate R2 hits from misses served by the origin through the Worker.
	switch resp.Header.Get("X-Cache") {
	case "HIT":
		return data, cacheHit, true, false
	case "MISS":
		return data, cacheMiss, true, false
	default:
		return nil, 0, false, false
	}
}

// rangeFramed reports whether value is exactly "bytes off-end/total" with a
// decimal total greater than end.
func rangeFramed(value string, off, end int64) bool {
	// Match the exact range, then accept only plain decimal digits as total.
	expected := "bytes " + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(end, 10) + "/"
	total, ok := strings.CutPrefix(value, expected)
	if !ok || total == "" || len(total) > 19 {
		return false
	}
	for _, r := range total {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(total, 10, 64)
	return err == nil && n > end
}

// cacheBreaker counts recent attempts and opens after a run or a high share
// of fallbacks. Reads hold its lock only to decide and to record.
type cacheBreaker struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// state is the breaker's state.
	state HTTPRangeCacheBreaker
	// consecutive counts fallbacks since the last success.
	consecutive int
	// windowStart is when the current window began.
	windowStart time.Time
	// attempts counts attempts in the current window.
	attempts int
	// fallbacks counts fallbacks in the current window.
	fallbacks int
	// openUntil ends the cool-off of an open breaker.
	openUntil time.Time
	// shown mirrors state for lock-free metrics.
	shown atomic.Int32
}

// admit reports whether a read made at now tries the cache, and whether it
// is the half-open breaker's single probe.
func (b *cacheBreaker) admit(now time.Time) (attempt, probe bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	switch b.state {
	case BreakerOpen:
		if now.Before(b.openUntil) {
			return false, false
		}
		b.set(BreakerHalfOpen)
		return true, true
	case BreakerHalfOpen:
		return false, false
	default:
		return true, false
	}
}

// record takes the outcome of an admitted attempt. Every fallback counts,
// 429 and 5xx answers included; a 429 (quota) opens the breaker at once for
// the longer cool-off. A neutral outcome, such as the caller cancelling, says
// nothing about the cache.
func (b *cacheBreaker) record(now time.Time, probe, ok, quota, neutral bool) {
	// Decide under the lock; outcomes from other attempts may interleave.
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// A spent quota opens a closed breaker, or reopens it after a probe.
	if quota {
		if probe || b.state == BreakerClosed {
			b.open(now, now.Add(breakerQuotaCoolOff))
		}
		return
	}

	// The probe alone decides a half-open breaker; a neutral probe lets the
	// next read probe again.
	if probe {
		switch {
		case neutral:
			b.open(now, now)
		case ok:
			b.reset(now)
			b.set(BreakerClosed)
		default:
			b.open(now, now.Add(breakerCoolOff))
		}
		return
	}

	// Attempts admitted before the breaker opened no longer count.
	if b.state != BreakerClosed || neutral {
		return
	}
	if now.Sub(b.windowStart) >= breakerWindow {
		b.windowStart, b.attempts, b.fallbacks = now, 0, 0
	}
	b.attempts++
	if ok {
		b.consecutive = 0
		return
	}
	b.consecutive++
	b.fallbacks++
	if b.consecutive >= breakerConsecutive || (b.attempts >= breakerWindowMin && 2*b.fallbacks > b.attempts) {
		b.open(now, now.Add(breakerCoolOff))
	}
}

// open opens the breaker until until and clears its counts.
func (b *cacheBreaker) open(now, until time.Time) {
	b.reset(now)
	b.openUntil = until
	b.set(BreakerOpen)
}

// reset clears the counts and starts a new window at now.
func (b *cacheBreaker) reset(now time.Time) {
	b.consecutive, b.attempts, b.fallbacks, b.windowStart = 0, 0, 0, now
}

// set changes the state and its lock-free mirror.
func (b *cacheBreaker) set(state HTTPRangeCacheBreaker) {
	b.state = state
	b.shown.Store(int32(state))
}
