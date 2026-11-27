//go:build !tinygo

package block_store_s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// guardWorker serves the range 3-6 of pfv1_test through fn, counting requests.
func guardWorker(t *testing.T, fn func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fn(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// guardHit writes a well framed hit for the range 3-6 with total.
func guardHit(w http.ResponseWriter, total string) {
	w.Header().Set("Content-Range", "bytes 3-6/"+total)
	w.Header().Set("Content-Length", "4")
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write([]byte("data"))
}

// guardRead reads the range 3-6 through cache and checks the bytes.
func guardRead(t *testing.T, ctx context.Context, cache *HTTPRangeCache, lower *cacheTestOrigin) {
	t.Helper()
	store := &ReadThroughStore{ObjectStore: lower, Cache: cache, Bucket: "world", PackPrefix: "test/packs/"}
	data, err := store.GetObjectRange(ctx, "world", "test/packs/pfv1_test", 3, 4)
	if err != nil || string(data) != "data" {
		t.Fatalf("read: %q %v", data, err)
	}
}

// TestHTTPRangeCacheFraming falls back on any Content-Range or Content-Length
// that does not describe exactly the requested bytes.
func TestHTTPRangeCacheFraming(t *testing.T) {
	cases := map[string]func(w http.ResponseWriter){
		"total-nonsense":  func(w http.ResponseWriter) { guardHit(w, "nonsense") },
		"total-empty":     func(w http.ResponseWriter) { guardHit(w, "") },
		"total-star":      func(w http.ResponseWriter) { guardHit(w, "*") },
		"total-short":     func(w http.ResponseWriter) { guardHit(w, "6") },
		"total-suffix":    func(w http.ResponseWriter) { guardHit(w, "10x") },
		"total-space":     func(w http.ResponseWriter) { guardHit(w, " 10") },
		"total-sign":      func(w http.ResponseWriter) { guardHit(w, "+10") },
		"range-trailing":  func(w http.ResponseWriter) { guardHit(w, "10, bytes 0-3/10") },
		"length-absent":   func(w http.ResponseWriter) { guardChunked(w) },
		"length-too-long": func(w http.ResponseWriter) { guardLength(w, "5") },
	}
	for name, respond := range cases {
		t.Run(name, func(t *testing.T) {
			srv, calls := guardWorker(t, func(w http.ResponseWriter, _ *http.Request) { respond(w) })
			lower := &cacheTestOrigin{}
			guardRead(t, t.Context(), NewHTTPRangeCache(srv.URL, "test-token"), lower)
			if calls.Load() != 1 || lower.calls.Load() != 1 {
				t.Fatalf("cache calls %d, direct calls %d: want 1, 1", calls.Load(), lower.calls.Load())
			}
		})
	}

	// The smallest valid total, one past the range's end, is a hit.
	srv, _ := guardWorker(t, func(w http.ResponseWriter, _ *http.Request) { guardHit(w, "7") })
	lower := &cacheTestOrigin{}
	guardRead(t, t.Context(), NewHTTPRangeCache(srv.URL, "test-token"), lower)
	if lower.calls.Load() != 0 {
		t.Fatal("valid framing fell back")
	}
}

// guardChunked writes a hit without a Content-Length.
func guardChunked(w http.ResponseWriter) {
	w.Header().Set("Content-Range", "bytes 3-6/10")
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(http.StatusPartialContent)
	w.(http.Flusher).Flush()
	_, _ = w.Write([]byte("data"))
}

// guardLength writes a hit of four bytes declaring length bytes, with the
// connection closed so the client sees the response end at the body.
func guardLength(w http.ResponseWriter, length string) {
	w.Header().Set("Content-Range", "bytes 3-6/10")
	w.Header().Set("Content-Length", length)
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write([]byte("data"))
}

// TestHTTPRangeCacheSlowBody falls back when the Worker sends headers at once
// and then trickles the body past the cache timeout.
func TestHTTPRangeCacheSlowBody(t *testing.T) {
	srv, calls := guardWorker(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 3-6/10")
		w.Header().Set("Content-Length", "4")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("da"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		_, _ = w.Write([]byte("ta"))
	})
	cache := NewHTTPRangeCache(srv.URL, "test-token")
	cache.client.Timeout = 50 * time.Millisecond
	lower := &cacheTestOrigin{}
	start := time.Now()
	guardRead(t, t.Context(), cache, lower)
	if calls.Load() != 1 || lower.calls.Load() != 1 {
		t.Fatalf("cache calls %d, direct calls %d: want 1, 1", calls.Load(), lower.calls.Load())
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("slow body held the read for %v", elapsed)
	}
}

// TestHTTPRangeCacheShortDeadline goes direct, without asking the Worker,
// when the caller's deadline leaves no room for an attempt and a fallback.
func TestHTTPRangeCacheShortDeadline(t *testing.T) {
	srv, calls := guardWorker(t, func(w http.ResponseWriter, _ *http.Request) { guardHit(w, "10") })
	cache := NewHTTPRangeCache(srv.URL, "test-token")
	lower := &cacheTestOrigin{}

	// 500 ms is less than the 750 ms timeout plus the margin.
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	guardRead(t, ctx, cache, lower)
	if calls.Load() != 0 || lower.calls.Load() != 1 {
		t.Fatalf("cache calls %d, direct calls %d: want 0, 1", calls.Load(), lower.calls.Load())
	}

	// A caller with time to spare still uses the cache.
	ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	guardRead(t, ctx, cache, lower)
	if calls.Load() != 1 || lower.calls.Load() != 1 {
		t.Fatalf("cache calls %d, direct calls %d: want 1, 1", calls.Load(), lower.calls.Load())
	}
}

// TestHTTPRangeCacheBreakerOpens stops asking a failing Worker after a run of
// fallbacks, and after a high share of fallbacks without such a run.
func TestHTTPRangeCacheBreakerOpens(t *testing.T) {
	// Five consecutive fallbacks: the sixth read goes direct without asking.
	srv, calls := guardWorker(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	cache := NewHTTPRangeCache(srv.URL, "test-token")
	lower := &cacheTestOrigin{}
	for range 6 {
		guardRead(t, t.Context(), cache, lower)
	}
	if calls.Load() != 5 || lower.calls.Load() != 6 {
		t.Fatalf("run: cache calls %d, direct calls %d: want 5, 6", calls.Load(), lower.calls.Load())
	}

	// Four failures then a success, repeated, never makes a run of five; the
	// share passes half once 20 attempts are in, at the 21st.
	var n atomic.Int32
	srv, calls = guardWorker(t, func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1)%5 == 0 {
			guardHit(w, "10")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	cache = NewHTTPRangeCache(srv.URL, "test-token")
	lower = &cacheTestOrigin{}
	for i := range 22 {
		guardRead(t, t.Context(), cache, lower)
		if want := int32(min(i+1, 21)); calls.Load() != want {
			t.Fatalf("share: read %s: cache calls %d, want %d", strconv.Itoa(i+1), calls.Load(), want)
		}
	}
}
