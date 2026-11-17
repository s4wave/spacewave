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

// TestEvictBoundedSweepHold runs bounded eviction against a pack store whose
// sweep holds the only pack of every block. While the pack is quarantined no
// block drops and the pass names the sweep; after a lift the pass drops
// within its budget. A quarantine after the drop leaves every block readable:
// the read of a dropped block falls back to the held pack and lifts it.
func TestEvictBoundedSweepHold(t *testing.T) {
	t.Cleanup(func() {
		evictBounded.Store(false)
		evictBudget.Store(nil)
	})

	// Upload blocks through a pack store that keeps its packs, with a local
	// index.
	f := newFallbackTest(t)
	backend, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), &account_settings.S3Location{
		Endpoint:   strings.TrimPrefix(f.srv.URL, "http://"),
		Region:     "us-east-1",
		Bucket:     "spaces",
		DisableSsl: true,
		KeepPacks:  true,
	}, "world-sweep", &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"}, hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(backend.Close)
	packs := backend.(*block_store_s3.PackStore)
	f.packs = backend
	const count, size = 8, 64
	var refs []*block.BlockRef
	for i := range count {
		data := make([]byte, size)
		copy(data, fmt.Appendf(nil, "sweep-%03d", i))
		ref, _, err := f.wb.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	f.upload()
	f.p.evictGate = &csync.Mutex{}
	f.p.remote = &atomic.Pointer[openBackend]{}
	f.p.remote.Store(&openBackend{store: backend})
	evictable := func() int64 {
		status, _ := f.wb.GetStatus()
		return status.EvictableBytes
	}

	// Find the one pack that holds the blocks.
	ids := make(map[string]struct{})
	for _, ref := range refs {
		locs, err := packs.LocatePacks(f.ctx, []*block.BlockRef{ref})
		if err != nil || len(locs) != 1 || locs[0] == "" {
			t.Fatalf("locate %s = %v, %v", ref.GetHash().MarshalString(), locs, err)
		}
		ids[locs[0]] = struct{}{}
	}
	if len(ids) != 1 {
		t.Fatalf("blocks in %d packs, want 1", len(ids))
	}
	var id string
	for id = range ids {
	}

	// Quarantined: nothing drops, and the pass names the sweep.
	SetEvictBudget(func() (block_store_writeback.EvictBudget, bool) {
		return block_store_writeback.EvictBudget{ID: "sweep", Bytes: count * size}, true
	})
	if err := packs.Quarantine(f.ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != 0 || result.Halted != block_store_s3.EvictHaltSweep || evictable() != count*size {
		t.Fatalf("quarantined: %+v, %v, evictable %d", result, err, evictable())
	}

	// Lifted: the pass drops every block.
	if err := packs.Lift(f.ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	if result, err := f.p.evict(f.ctx, 1<<20); err != nil || result.EvictedBytes != count*size || evictable() != 0 {
		t.Fatalf("lifted: %+v, %v, evictable %d", result, err, evictable())
	}

	// Quarantined after the drop: each block still reads, through the
	// fallback, which lifts the pack.
	if err := packs.Quarantine(f.ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		data, ok, err := f.reads.GetBlock(f.ctx, ref)
		if err != nil || !ok || len(data) != size {
			t.Fatalf("read %s after the quarantine: %d bytes, %v, %v", ref.GetHash().MarshalString(), len(data), ok, err)
		}
	}
	if !packs.Registered(id) {
		t.Fatal("a read of a dropped block did not lift its quarantined pack")
	}
}
