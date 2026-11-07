package cdn_rootserve

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
)

// testSpaceID is the Space the tests serve.
const testSpaceID = "01kpn3x0y79yr94ps1yae206vp"

// serve mounts h at a test server and returns its base URL.
func serve(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

// get fetches path with an optional If-None-Match.
func get(t *testing.T, url, etag string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestOwnedPointerRevalidates proves an owned pointer is served with no-cache
// and a strong ETag, answers a matching If-None-Match with 304, and a reader
// sees a new commit on its next fetch.
func TestOwnedPointerRevalidates(t *testing.T) {
	// Serve an owned Space at seqno 1.
	h := NewHandler("", nil)
	if err := h.SetOwned(&cdn.CdnRootPointer{SpaceId: testSpaceID, ConfigChainSeqno: 1}); err != nil {
		t.Fatal(err)
	}
	base := serve(t, h)
	url := base + "/" + testSpaceID + "/" + PointerName

	// The first read carries the cache rule and a tag.
	resp := get(t, url, "")
	etag := resp.Header.Get("ETag")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != PointerCacheControl || etag == "" || etag[0] != '"' {
		t.Fatalf("first read: %d %q %q", resp.StatusCode, resp.Header.Get("Cache-Control"), etag)
	}

	// A read with the tag is answered 304.
	if resp := get(t, url, etag); resp.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidation: %d", resp.StatusCode)
	}

	// A commit moves the pointer, and the reader decodes the new one.
	if err := h.SetOwned(&cdn.CdnRootPointer{SpaceId: testSpaceID, ConfigChainSeqno: 2}); err != nil {
		t.Fatal(err)
	}
	if resp := get(t, url, etag); resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") == etag {
		t.Fatalf("after commit: %d %q", resp.StatusCode, resp.Header.Get("ETag"))
	}
	ptr, err := cdn_bstore.FetchRootPointer(t.Context(), nil, base, testSpaceID)
	if err != nil || ptr.GetConfigChainSeqno() != 2 {
		t.Fatalf("reader: %v %v", ptr, err)
	}

	// An unknown Space and a malformed path are 404, a write is refused.
	if resp := get(t, base+"/othertestspace/"+PointerName, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown space: %d", resp.StatusCode)
	}
	if resp := get(t, base+"/../"+PointerName, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bad path: %d", resp.StatusCode)
	}
	post, err := http.Post(url, "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("post: %d", post.StatusCode)
	}
}

// TestForeignPointerKeepsLastCopy proves a foreign pointer is revalidated at
// the origin with its held tag, served from the held copy on 304, kept when
// the origin fails, and dropped when the origin reports it gone.
func TestForeignPointerKeepsLastCopy(t *testing.T) {
	// Serve a foreign pointer from an origin that can fail or forget it.
	enc, err := EncodePointer(&cdn.CdnRootPointer{SpaceId: testSpaceID, ConfigChainSeqno: 7})
	if err != nil {
		t.Fatal(err)
	}
	var status atomic.Int32
	status.Store(http.StatusOK)
	var conditional atomic.Int32
	origin := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code := int(status.Load()); code != http.StatusOK {
			http.Error(w, "origin down", code)
			return
		}
		if r.Header.Get("If-None-Match") == enc.ETag {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(enc.Body)
	}))
	var fetches atomic.Int32
	counted := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		http.Redirect(w, r, origin+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	h := NewHandler(counted, nil)
	clock := time.Unix(1, 0)
	h.now = func() time.Time { return clock }
	base := serve(t, h)
	read := func(want uint64) {
		t.Helper()
		ptr, err := cdn_bstore.FetchRootPointer(t.Context(), nil, base, testSpaceID)
		if err != nil || ptr.GetConfigChainSeqno() != want {
			t.Fatalf("read: %v %v", ptr, err)
		}
	}

	// The first read fetches it; reads within a second are served held.
	read(7)
	read(7)
	if fetches.Load() != 1 {
		t.Fatalf("origin fetches within a second: %d, want 1", fetches.Load())
	}

	// After a second the held copy is revalidated with its tag.
	clock = clock.Add(revalidateAfter)
	read(7)
	if conditional.Load() != 1 || fetches.Load() != 2 {
		t.Fatalf("conditional %d fetches %d, want 1 and 2", conditional.Load(), fetches.Load())
	}

	// A failing origin leaves the last copy served.
	status.Store(http.StatusBadGateway)
	clock = clock.Add(revalidateAfter)
	read(7)

	// An origin that forgets the Space drops it.
	status.Store(http.StatusNotFound)
	clock = clock.Add(revalidateAfter)
	if ptr, err := cdn_bstore.FetchRootPointer(t.Context(), nil, base, testSpaceID); err != nil || ptr != nil {
		t.Fatalf("origin forgot: %v %v", ptr, err)
	}

	// With no held copy a failing origin is a 502.
	status.Store(http.StatusBadGateway)
	if resp := get(t, base+"/"+testSpaceID+"/"+PointerName, ""); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("no copy: %d", resp.StatusCode)
	}
}

// TestImmutablePacks proves packs carry the year-long rule and other objects
// do not.
func TestImmutablePacks(t *testing.T) {
	base := serve(t, ImmutablePacks(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})))
	if got := get(t, base+"/"+testSpaceID+"/packs/ab/abc.kvf", "").Header.Get("Cache-Control"); got != PackCacheControl {
		t.Fatalf("pack: %q", got)
	}
	if got := get(t, base+"/"+testSpaceID+"/other", "").Header.Get("Cache-Control"); got != "" {
		t.Fatalf("other: %q", got)
	}
}
