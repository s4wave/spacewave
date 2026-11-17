package provider_local

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/csync"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// TestEvictBoundedProvider checks that once SetEvictBudget is called, the
// placed store's passes are bounded: without a budget, or over a backend that
// cannot verify its packs, nothing drops, though the backend holds every
// block; with one, the pass drops within it, and a read refills.
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
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || result.Halted != evictHaltCompaction || evictable() != count*size {
		t.Fatalf("deleting backend: %+v, %v, evictable %d", result, err, evictable())
	}

	// The budget over the pack store drops at most its bytes.
	f.p.remote.Store(&openBackend{store: f.packs})
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

// deletingPacks is a pack store that reports compaction and reclaim enabled.
type deletingPacks struct {
	backendStore
	packVerifyStore
}

// KeepsBlocks reports that the store deletes its own packs.
func (deletingPacks) KeepsBlocks() bool { return false }
