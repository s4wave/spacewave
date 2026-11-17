//go:build !tinygo

package block_store_s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// cacheTestOrigin counts the direct reads made after an optional cache attempt.
type cacheTestOrigin struct {
	ObjectStore
	calls atomic.Int32
}

// GetObjectRange serves the fake origin's bytes.
func (o *cacheTestOrigin) GetObjectRange(context.Context, string, string, int64, int) ([]byte, error) {
	o.calls.Add(1)
	return []byte("data"), nil
}

// TestHTTPRangeCache exercises cache outcomes and atomic fallback of bodies.
func TestHTTPRangeCache(t *testing.T) {
	// Never send a private token over a cleartext network connection.
	if NewHTTPRangeCache("http://cache.example", "test-token") != nil {
		t.Fatal("cleartext remote cache accepted")
	}

	// Exercise each response independently with a real HTTP client.
	for _, name := range []string{"hit", "miss", "timeout", "wrong-status", "unset", "short-body", "wrong-range", "redirect"} {
		t.Run(name, func(t *testing.T) {
			// Serve only the request shape the Worker expects.
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Range") != "bytes=3-6" || r.URL.Path != "/world/test/packs/pfv1_test" {
					t.Error("unexpected cache request")
				}
				if name == "timeout" {
					<-r.Context().Done()
					return
				}
				if name == "wrong-status" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if name == "redirect" {
					w.Header().Set("Location", "/never")
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				w.Header().Set("Content-Range", "bytes 3-6/10")
				w.Header().Set("X-Cache", "HIT")
				if name == "miss" {
					w.Header().Set("X-Cache", "MISS")
				}
				if name == "wrong-range" {
					w.Header().Set("Content-Range", "bytes 0-3/10")
				}
				w.WriteHeader(http.StatusPartialContent)
				if name == "short-body" {
					_, _ = w.Write([]byte("bad"))
					return
				}
				_, _ = w.Write([]byte("data"))
			}))
			defer srv.Close()

			// Bound the timeout test without changing the production default.
			cache := NewHTTPRangeCache(srv.URL, "test-token")
			if name == "timeout" {
				cache.client.Timeout = 20 * time.Millisecond
			}
			if name == "unset" {
				cache = NewHTTPRangeCache("", "")
			}
			lower := &cacheTestOrigin{}
			store := &ReadThroughStore{ObjectStore: lower, Cache: cache, Bucket: "world", PackPrefix: "test/packs/"}
			data, err := store.GetObjectRange(t.Context(), "world", "test/packs/pfv1_test", 3, 4)
			if err != nil || string(data) != "data" {
				t.Fatalf("read: %q %v", data, err)
			}

			// Check the source counters as well as the returned bytes.
			fallback := name != "hit" && name != "miss"
			if (lower.calls.Load() != 0) != fallback {
				t.Fatalf("direct calls: %d", lower.calls.Load())
			}
			if name == "unset" {
				if calls.Load() != 0 {
					t.Fatal("disabled cache called")
				}
				return
			}
			counts := cache.Counts()
			source := 0
			if name == "miss" {
				source = 1
			}
			if fallback {
				source = 2
			}
			if counts.Requests[source] != 1 || counts.Bytes[source] != 4 {
				t.Fatalf("counts: %+v", counts)
			}
			if calls.Load() != 1 {
				t.Fatalf("cache calls: %d", calls.Load())
			}
		})
	}
}

// TestHTTPRangeCacheBreakerProbe covers the cool-off, the single half-open
// probe and the counts and state the backend exports.
func TestHTTPRangeCacheBreakerProbe(t *testing.T) {
	// Serve failures or hits, holding a request while hold is set.
	var failing atomic.Bool
	var hold atomic.Pointer[chan struct{}]
	arrived := make(chan struct{}, 1)
	failing.Store(true)
	srv, calls := guardWorker(t, func(w http.ResponseWriter, _ *http.Request) {
		if h := hold.Load(); h != nil {
			arrived <- struct{}{}
			<-*h
		}
		if failing.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		guardHit(w, "10")
	})
	cache := NewHTTPRangeCache(srv.URL, "test-token")
	var clock atomic.Int64
	clock.Store(time.Unix(1_800_000_000, 0).UnixNano())
	cache.now = func() time.Time { return time.Unix(0, clock.Load()) }
	lower := &cacheTestOrigin{}
	check := func(step string, cacheCalls int32, state HTTPRangeCacheBreaker, requests [6]uint64) {
		t.Helper()
		counts := cache.Counts()
		if calls.Load() != cacheCalls || counts.Breaker != state || counts.Requests != requests {
			t.Fatalf("%s: cache calls %d, counts %+v", step, calls.Load(), counts)
		}
	}

	// Open after five fallbacks; reads in the cool-off are skipped.
	for range 5 {
		guardRead(t, t.Context(), cache, lower)
	}
	check("open", 5, BreakerOpen, [6]uint64{0, 0, 5, 0, 0, 0})
	clock.Add(int64(breakerCoolOff - time.Second))
	guardRead(t, t.Context(), cache, lower)
	check("cool-off", 5, BreakerOpen, [6]uint64{0, 0, 5, 1, 0, 0})

	// A failed probe opens it for another cool-off.
	clock.Add(int64(time.Second))
	guardRead(t, t.Context(), cache, lower)
	check("failed probe", 6, BreakerOpen, [6]uint64{0, 0, 6, 1, 0, 0})
	guardRead(t, t.Context(), cache, lower)
	check("reopened", 6, BreakerOpen, [6]uint64{0, 0, 6, 2, 0, 0})

	// While a probe is in flight, other reads go direct.
	clock.Add(int64(breakerCoolOff))
	failing.Store(false)
	release := make(chan struct{})
	hold.Store(&release)
	done := make(chan struct{})
	go func() {
		defer close(done)
		store := &ReadThroughStore{ObjectStore: lower, Cache: cache, Bucket: "world", PackPrefix: "test/packs/"}
		if data, err := store.GetObjectRange(t.Context(), "world", "test/packs/pfv1_test", 3, 4); err != nil || string(data) != "data" {
			t.Errorf("probe read: %q %v", data, err)
		}
	}()
	<-arrived
	hold.Store(nil)
	if cache.Counts().Breaker != BreakerHalfOpen {
		t.Fatal("probe not half-open")
	}
	guardRead(t, t.Context(), cache, lower)
	close(release)
	<-done

	// The probe's hit closes the breaker, and reads use the cache again.
	check("probe", 7, BreakerClosed, [6]uint64{1, 0, 6, 3, 0, 0})
	guardRead(t, t.Context(), cache, lower)
	check("closed", 8, BreakerClosed, [6]uint64{2, 0, 6, 3, 0, 0})
	if lower.calls.Load() != 9 {
		t.Fatalf("direct calls %d", lower.calls.Load())
	}
}

// TestHTTPRangeCacheBreakerCancel does not count a caller's cancellation
// against the Worker.
func TestHTTPRangeCacheBreakerCancel(t *testing.T) {
	srv, calls := guardWorker(t, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	cache := NewHTTPRangeCache(srv.URL, "test-token")
	store := &ReadThroughStore{ObjectStore: &cacheTestOrigin{}, Cache: cache, Bucket: "world", PackPrefix: "test/packs/"}
	for range breakerConsecutive + 1 {
		ctx, cancel := context.WithCancel(t.Context())
		timer := time.AfterFunc(10*time.Millisecond, cancel)
		_, _ = store.GetObjectRange(ctx, "world", "test/packs/pfv1_test", 3, 4)
		timer.Stop()
		cancel()
	}
	if calls.Load() != breakerConsecutive+1 || cache.Counts().Breaker != BreakerClosed {
		t.Fatalf("cache calls %d, state %d", calls.Load(), cache.Counts().Breaker)
	}
}

// TestHTTPRangeCacheBreakerQuota opens the breaker on one 429 from the Worker
// for the longer quota cool-off.
func TestHTTPRangeCacheBreakerQuota(t *testing.T) {
	var quota atomic.Bool
	quota.Store(true)
	srv, calls := guardWorker(t, func(w http.ResponseWriter, _ *http.Request) {
		if quota.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		guardHit(w, "10")
	})
	cache := NewHTTPRangeCache(srv.URL, "test-token")
	var clock atomic.Int64
	clock.Store(time.Unix(1_800_000_000, 0).UnixNano())
	cache.now = func() time.Time { return time.Unix(0, clock.Load()) }
	lower := &cacheTestOrigin{}

	// One 429 opens it; the ordinary cool-off is not enough to probe.
	guardRead(t, t.Context(), cache, lower)
	if calls.Load() != 1 || cache.Counts().Breaker != BreakerOpen {
		t.Fatalf("after 429: cache calls %d, state %d", calls.Load(), cache.Counts().Breaker)
	}
	clock.Add(int64(breakerCoolOff))
	guardRead(t, t.Context(), cache, lower)
	if calls.Load() != 1 {
		t.Fatalf("probed inside the quota cool-off: cache calls %d", calls.Load())
	}

	// After the quota cool-off a probe that hits closes it.
	clock.Add(int64(breakerQuotaCoolOff - breakerCoolOff))
	quota.Store(false)
	guardRead(t, t.Context(), cache, lower)
	counts := cache.Counts()
	if calls.Load() != 2 || counts.Breaker != BreakerClosed || counts.Requests != [6]uint64{1, 0, 1, 1, 0, 0} {
		t.Fatalf("after probe: cache calls %d, counts %+v", calls.Load(), counts)
	}
}
