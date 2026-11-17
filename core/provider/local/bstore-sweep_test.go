package provider_local

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// rootedStore is an ownedStore that records the bucket's named roots, pins and
// stages in the Volume's ownership graph, as the account's bucket handle does
// on a Volume with atomic publication.
type rootedStore struct {
	ownedStore
}

// SupportsRootRetention reports that the bucket records its roots.
func (r rootedStore) SupportsRootRetention() bool { return true }

// SetRetainedRoots writes the entries and moves the bucket's named roots.
func (r rootedStore) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return r.SetBucketRoots(ctx, "bucket", entries, roots)
}

// PinRoot pins a reader's root in the graph.
func (r rootedStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return r.PinBucketRoot(ctx, ref)
}

// OpenStage writes through the bucket; the test opens no stage.
func (r rootedStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	return r, func() {}, nil
}

// ReleaseRoots drops the bucket's staging edges to roots.
func (r rootedStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return r.ReleaseBucketRoots(ctx, "bucket", refs)
}

// TestSweepRootsRetention lists the roots of a write-back store mounted over a
// Volume's bucket: the World engine's replay-base and replay-span roots and a
// reader's pin, each set through the write-back as the engine sets them. Over
// a packahead log, which records no roots in the graph, the same writes leave
// the graph without them, and the listing must fail instead of returning a
// short list the sweep would trust.
func TestSweepRootsRetention(t *testing.T) {
	// Open a Volume whose graph roots the bucket, and a write-back over it.
	ctx := t.Context()
	local, vol := newOwnedVolume(t)
	if err := vol.GetRefGraph().AddRef(ctx, block_gc.NodeGCRoot, block_gc.BucketIRI("bucket")); err != nil {
		t.Fatal(err)
	}
	mount := func(id string, store block.StoreOps, packed *block_store_writeback.PackedStore) (*block_store_writeback.Store, *sweepRoots) {
		t.Helper()
		markers, release, err := vol.AccessObjectStore(ctx, "test/markers/"+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		var wb *block_store_writeback.Store
		if packed != nil {
			wb, err = block_store_writeback.NewStoreWithPacks(ctx, packed, vol, markers, "test/markers/"+id, false)
		} else {
			wb, err = block_store_writeback.NewStore(ctx, store, vol, markers, "test/markers/"+id, false)
		}
		if err != nil {
			t.Fatal(err)
		}
		key := adoptKey{volumeID: "test/sweep-roots", storeID: id}
		s := lookupSweep(key)
		s.mtx.Lock()
		s.wb, s.roots, s.bucket = wb, vol.GetRefGraph(), "bucket"
		s.mtx.Unlock()
		return wb, &sweepRoots{key: key}
	}

	// Set the engine's replay roots and pin a reader's root, as the engine does.
	setRoots := func(wb *block_store_writeback.Store, tag string) []string {
		t.Helper()
		var refs []string
		var entries []*block.PutBatchEntry
		for _, data := range []string{tag + " replay base", tag + " replay span", tag + " pinned"} {
			ref, err := block.BuildBlockRef([]byte(data), nil)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: []byte(data), RefsKnown: true})
			refs = append(refs, ref.MarshalString())
		}
		roots := []block.NamedRoot{{Name: "replay-base", Ref: entries[0].Ref}, {Name: "replay-span", Ref: entries[1].Ref}}
		if err := block.SetRetainedRoots(ctx, wb, entries[:2], roots); err != nil {
			t.Fatal(err)
		}
		if _, err := wb.PutBlockBatch(ctx, entries[2:]); err != nil {
			t.Fatal(err)
		}
		release, err := block.PinRoot(ctx, wb, entries[2].Ref)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		return refs
	}

	// A store that records its roots lists both replay roots and the pin.
	wb, roots := mount("rooted", rootedStore{local}, nil)
	want := setRoots(wb, "rooted")
	var got []string
	err := roots.ListRoots(ctx, "", func(root block.NamedRoot) error {
		got = append(got, root.Ref.MarshalString())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range want {
		if !slices.Contains(got, ref) {
			t.Fatalf("roots %v miss %s", got, ref)
		}
	}

	// A store over a packahead log records no roots, so the listing fails.
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	packedWB, packedRoots := mount("packed", nil, block_store_writeback.NewPackedStore(log, rootedStore{local}, false))
	setRoots(packedWB, "packed")
	err = packedRoots.ListRoots(ctx, "", func(block.NamedRoot) error { return nil })
	if err == nil {
		t.Fatal("a store that records no roots listed them without an error")
	}
}
