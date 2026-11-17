//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// rejectWorker serves pack ranges as the Worker does, from the fake bucket.
// While armed, it swaps the bytes of target for wrong bytes of the same length
// and counts the requests it answers.
type rejectWorker struct {
	// armed turns on the swap and the counts.
	armed atomic.Bool
	// target is the block data to swap.
	target []byte
	// calls counts armed requests.
	calls atomic.Int32
	// late counts armed requests after the cache counted a rejected read.
	late atomic.Int32
	// cache is the cache reading through the Worker.
	cache *HTTPRangeCache
}

// newRejectStore adopts three blocks into a store whose pack range reads go
// through a Worker that swaps the data of block target, and arms the Worker.
func newRejectStore(t *testing.T, target int) (*PackStore, []*block.BlockRef, *rejectWorker) {
	// Serve the Worker's answers from the fake bucket's true bytes.
	t.Helper()
	_, client := newFakeBucket(t, nil)
	w := &rejectWorker{target: []byte("w block " + strconv.Itoa(target))}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Read the true range from the bucket.
		rng := strings.TrimPrefix(r.Header.Get("Range"), "bytes=")
		first, last, _ := strings.Cut(rng, "-")
		off, _ := strconv.ParseInt(first, 10, 64)
		end, _ := strconv.ParseInt(last, 10, 64)
		data, err := client.GetObjectRange(r.Context(), "bucket", strings.TrimPrefix(r.URL.Path, "/bucket/"), off, int(end-off+1))
		if err != nil || len(data) != int(end-off+1) {
			rw.WriteHeader(http.StatusBadGateway)
			return
		}

		// Swap the target's bytes while armed, framed as the Worker frames.
		if w.armed.Load() {
			w.calls.Add(1)
			if w.cache.Counts().Requests[cacheRejected] != 0 {
				w.late.Add(1)
			}
			data = bytes.ReplaceAll(data, w.target, bytes.ToUpper(w.target))
		}
		rw.Header().Set("Content-Range", "bytes "+first+"-"+last+"/"+strconv.FormatInt(end+1, 10))
		rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
		rw.Header().Set("X-Cache", "HIT")
		rw.WriteHeader(http.StatusPartialContent)
		_, _ = rw.Write(data)
	}))
	t.Cleanup(srv.Close)

	// Read the store's packs through the Worker; arm it once the blocks are in.
	w.cache = NewHTTPRangeCache(srv.URL, "test-token")
	objects := &ReadThroughStore{ObjectStore: client, Cache: w.cache, Bucket: "bucket", PackPrefix: "p/" + packDir}
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	s := NewPackStore(logrus.NewEntry(logrus.New()), objects, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
	t.Cleanup(s.Close)
	refs := adoptBlocks(t.Context(), t, s, "w", 3)
	w.armed.Store(true)
	return s, refs, w
}

// checkRejected checks a read of block target that the Worker answered with
// wrong bytes: the read returns the bucket's bytes, the cache counts a
// rejected read, and the Worker is not asked again after the rejection.
func checkRejected(t *testing.T, s *PackStore, refs []*block.BlockRef, w *rejectWorker, target int) {
	// Read the block and check where its bytes came from.
	t.Helper()
	data, found, err := s.GetBlock(t.Context(), refs[target])
	if err != nil || !found || !bytes.Equal(data, w.target) {
		t.Errorf("read = %q, %v, %v, want %q from the bucket", data, found, err, w.target)
	}
	if w.calls.Load() == 0 {
		t.Error("the Worker was never asked")
	}
	if late := w.late.Load(); late != 0 {
		t.Errorf("the Worker was asked %d times after its bytes were rejected", late)
	}
	if n := w.cache.Counts().Requests[cacheRejected]; n == 0 {
		t.Errorf("rejected reads %d, want at least 1; counts %+v", n, w.cache.Counts())
	}
}

// TestPackStoreCacheRejectExtent reads a block's extent from the bucket after
// the Worker's copy failed the block's hash, asking the Worker once.
func TestPackStoreCacheRejectExtent(t *testing.T) {
	// One extent read asks the Worker, then the bucket.
	SetIndexExtentReads(true)
	s, refs, w := newRejectStore(t, 0)
	checkRejected(t, s, refs, w, 0)
	if calls := w.calls.Load(); calls != 1 {
		t.Errorf("Worker asked %d times, want 1", calls)
	}
	if n := w.cache.Counts().Requests[cacheRejected]; n != 1 {
		t.Errorf("rejected reads %d, want 1", n)
	}
}

// TestPackStoreCacheRejectWindow reads a block through the pack readers from
// the bucket after the Worker's window failed the block's hash.
func TestPackStoreCacheRejectWindow(t *testing.T) {
	// The pack readers' window comes from the Worker, then the bucket.
	SetIndexExtentReads(false)
	defer SetIndexExtentReads(true)
	s, refs, w := newRejectStore(t, 1)
	checkRejected(t, s, refs, w, 1)
	t.Logf("Worker asked %d times; counts %+v", w.calls.Load(), w.cache.Counts())
}
