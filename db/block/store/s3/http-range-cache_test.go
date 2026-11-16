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
