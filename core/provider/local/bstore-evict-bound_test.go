package provider_local

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/csync"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/sirupsen/logrus"
)

// TestEvictBoundedProvider checks that once SetEvictBudget is called, the
// placed store's passes are bounded: without a budget, over a backend that
// cannot verify its packs, or over packs whose bucket entry is missing or
// does not name their blocks, nothing drops, though the backend holds every
// block; a failed entry GET drops nothing and keeps no refusal; with a budget,
// the pass drops within it, and a read refills.
func TestEvictBoundedProvider(t *testing.T) {
	t.Cleanup(func() {
		evictBounded.Store(false)
		evictBudget.Store(nil)
	})

	// Upload blocks through a pack store with a local index.
	f := newFallbackTest(t)
	indexed, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), &account_settings.S3Location{
		Endpoint:   strings.TrimPrefix(f.srv.URL, "http://"),
		Region:     "us-east-1",
		Bucket:     "spaces",
		DisableSsl: true,
		KeepPacks:  true,
	}, "world-bound", &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"}, hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(indexed.Close)
	f.packs = indexed
	const count, size = 8, 64
	var refs []*block.BlockRef
	for i := range count {
		data := make([]byte, size)
		copy(data, fmt.Appendf(nil, "bound-%03d", i))
		ref, _, err := f.wb.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	f.upload()
	f.p.evictGate = &csync.Mutex{}
	f.p.remote = &atomic.Pointer[openBackend]{}
	evictable := func() int64 {
		status, _ := f.wb.GetStatus()
		return status.EvictableBytes
	}
	if got := evictable(); got != count*size {
		t.Fatalf("evictable = %d after upload, want %d", got, count*size)
	}

	// Bounded without a budget: nothing drops.
	f.p.remote.Store(&openBackend{store: f.packs})
	SetEvictBudget(nil)
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || evictable() != count*size {
		t.Fatalf("no budget: %+v, %v, evictable %d", result, err, evictable())
	}

	// A budget over a backend that cannot verify: nothing drops.
	SetEvictBudget(func() (block_store_writeback.EvictBudget, bool) {
		return block_store_writeback.EvictBudget{ID: "b1", Bytes: 3 * size}, true
	})
	f.p.remote.Store(&openBackend{store: &evictTestBackend{backendStore: f.packs, failed: true}})
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || evictable() != count*size {
		t.Fatalf("unverifiable backend: %+v, %v, evictable %d", result, err, evictable())
	}

	// A budget over a pack store that compacts and reclaims: nothing drops,
	// and the pass names the delete path.
	f.p.remote.Store(&openBackend{store: deletingPacks{backendStore: f.packs, packVerifyStore: f.packs.(packVerifyStore)}})
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || result.Halted != block_store_s3.EvictHaltCompaction || evictable() != count*size {
		t.Fatalf("deleting backend: %+v, %v, evictable %d", result, err, evictable())
	}

	// A pack a reader with no local index cannot find keeps every block:
	// first with its bucket entry gone, then with an entry whose bloom filter
	// lacks the pack's blocks, though the local index names the pack and its
	// bytes verify.
	f.p.remote.Store(&openBackend{store: f.packs})
	entries := f.entryObjects()
	if len(entries) == 0 {
		t.Fatal("no pack entry in the bucket")
	}
	otherBloom := bloomOf(t, []byte("not a block of the pack"))
	for name, lie := range map[string]func(key string, data []byte){
		"no entry": func(key string, _ []byte) { delete(f.bucket.objects, key) },
		"bloom lacks the blocks": func(key string, data []byte) {
			entry := &packfile.PackfileEntry{}
			if err := entry.UnmarshalVT(data); err != nil {
				t.Fatal(err)
			}
			entry.BloomFilter = otherBloom
			lied, err := entry.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			f.bucket.objects[key] = lied
		},
	} {
		// Make the bucket lie about each entry.
		f.bucket.mtx.Lock()
		for key, data := range entries {
			lie(key, data)
		}
		f.bucket.mtx.Unlock()

		// Nothing drops, and the refusals name the entry.
		before := refusedEntry(f)
		if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || evictable() != count*size || refusedEntry(f) == before {
			t.Fatalf("%s: %+v, %v, evictable %d, entry refusals %d", name, result, err, evictable(), refusedEntry(f))
		}

		// Put the true entries back.
		f.bucket.mtx.Lock()
		maps.Copy(f.bucket.objects, entries)
		f.bucket.mtx.Unlock()
	}

	// A failed GET of each entry refuses the packs as a fetch failure, which
	// keeps no refusal: the next pass fetches and verifies them.
	f.bucket.mtx.Lock()
	f.bucket.failGet = func(key string) bool { return strings.Contains(key, "/entries/") }
	f.bucket.mtx.Unlock()
	before := f.status().Bound.Refused[block_store_s3.VerifyFetch]
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || evictable() != count*size || f.status().Bound.Refused[block_store_s3.VerifyFetch] == before {
		t.Fatalf("failed entry GET: %+v, %v, evictable %d, refusals %v", result, err, evictable(), f.status().Bound.Refused)
	}
	f.bucket.mtx.Lock()
	f.bucket.failGet = nil
	f.bucket.mtx.Unlock()

	// The budget over the pack store drops at most its bytes.
	result, err := f.p.evict(f.ctx, 1<<20)
	if err != nil || result.EvictedBytes == 0 || result.EvictedBytes > 3*size {
		t.Fatalf("budget: %+v, %v", result, err)
	}
	status, _ := f.wb.GetStatus()
	if !status.Bound.BudgetKnown || status.Bound.BudgetLeft != 3*size-result.EvictedBytes {
		t.Fatalf("bound status = %+v after dropping %d", status.Bound, result.EvictedBytes)
	}

	// Every block still reads, a dropped one from the packs.
	for _, ref := range refs {
		data, ok, err := f.reads.GetBlock(f.ctx, ref)
		if err != nil || !ok || len(data) != size {
			t.Fatalf("read %s: %d bytes, %v, %v", ref.GetHash().MarshalString(), len(data), ok, err)
		}
	}
}

// entryObjects returns a copy of every pack entry object in the bucket, by
// key.
func (f *fallbackTest) entryObjects() map[string][]byte {
	f.bucket.mtx.Lock()
	defer f.bucket.mtx.Unlock()
	out := make(map[string][]byte)
	for key, data := range f.bucket.objects {
		if strings.Contains(key, "/entries/") {
			out[key] = bytes.Clone(data)
		}
	}
	return out
}

// refusedEntry returns the bounded passes' refusals for a pack's entry.
func refusedEntry(f *fallbackTest) int64 {
	status, _ := f.wb.GetStatus()
	return status.Bound.Refused[block_store_s3.VerifyEntry]
}

// bloomOf returns an encoded bloom filter holding only keys.
func bloomOf(t *testing.T, keys ...[]byte) []byte {
	bf := writer.DefaultPolicy().NewBloomFilter(uint64(len(keys)))
	for _, k := range keys {
		bf.Add(k)
	}
	data, err := bloom.NewBloom(bf).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// deletingPacks is a pack store that reports compaction and reclaim enabled.
type deletingPacks struct {
	backendStore
	packVerifyStore
}

// EvictHalt reports that the store deletes its own packs.
func (deletingPacks) EvictHalt() string { return block_store_s3.EvictHaltCompaction }
