package provider_local

import (
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
)

// TestPinRootUnknownRefs checks that fetching an untracked root cannot write it
// into the destination as a leaf.
func TestPinRootUnknownRefs(t *testing.T) {
	// Serve an untracked block from a store that retains only its bytes.
	ctx := t.Context()
	source := newBatchForwardTestStore()
	ref, _, err := source.PutBlock(ctx, []byte("untracked root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	destination := newBatchForwardTestStore()
	store := &BlockStore{store: destination, readStore: source}

	// Refuse the pin before writing a block with unknown references.
	release, err := store.PinRoot(ctx, ref)
	if release != nil {
		release()
	}
	if !errors.Is(err, block.ErrRefsUnknown) || !strings.Contains(err.Error(), ref.MarshalString()) {
		t.Fatalf("pin = %v, want ErrRefsUnknown naming %s", err, ref.MarshalString())
	}
	found, err := destination.GetBlockExists(ctx, ref)
	if err != nil || found {
		t.Fatalf("destination = %v/%v, want absent", found, err)
	}
}

// TestSetRootsBringsRemoteRootBlock checks on a Volume that frees with a 1ns
// grace that a root moving onto a block only the read path holds, as a
// follower's replay base on another device's World does, stores the block with
// its refs in the same change, so collection keeps it. The test also checks
// that a block the read path holds without refs refuses the move, and that a
// pin on such a block stores it under a stage, which the Volume takes where
// the Volume refuses an unheld put, so collection keeps the block while
// pinned.
func TestSetRootsBringsRemoteRootBlock(t *testing.T) {
	// Serve a root block with a child from the read path only.
	ctx := t.Context()
	source := block_store.NewStore("source", block_mock.NewRefStore(newBatchForwardTestStore()))
	child, _, err := source.PutBlock(ctx, []byte("remote child"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := source.PutBlock(ctx, []byte("remote root"), &block.PutOpts{Refs: []*block.BlockRef{child}, RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	vol, err := volume_kvtx.NewVolume(ctx, "test/follower", store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(),
		&store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	store := &BlockStore{store: block_store.NewStore("dest", vol), readStore: source}

	// The move stores the root block locally, and collection keeps it.
	if err := store.SetRoots(ctx, nil, []block.NamedRoot{{Name: "replay-base", Ref: root}}); err != nil {
		t.Fatal(err)
	}
	if _, err := vol.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := vol.GetStoredBlock(ctx, root)
	if err != nil || stored == nil || !stored.RefsSource.Recorded() || len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(child) {
		t.Fatalf("local root block %v, %v; want it stored with its child", stored, err)
	}

	// A block without refs refuses the move and leaves the root.
	bare, _, err := source.PutBlock(ctx, []byte("bare root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = store.SetRoots(ctx, nil, []block.NamedRoot{{Name: "replay-base", Ref: bare, Previous: root}})
	if !errors.Is(err, block.ErrRefsUnknown) {
		t.Fatalf("move onto a bare block = %v, want ErrRefsUnknown", err)
	}
	if held, err := vol.GetRoot(ctx, "replay-base"); err != nil || !held.EqualsRef(root) {
		t.Fatalf("root after the refused move = %v, %v; want the first root", held, err)
	}
	// A pin stores the block it lacks under a stage, then holds it.
	leaf, _, err := source.PutBlock(ctx, []byte("remote pinned"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	bkt := volume_controller.NewBucketStore(vol, &bucket.Config{Id: "dest", Rev: 1})
	pinned := &BlockStore{store: block_store.NewStore("dest", bkt), readStore: source}
	release, err := pinned.PinRoot(ctx, leaf)
	if err != nil {
		t.Fatalf("pin of a block only the read path holds: %v", err)
	}
	defer release()
	if _, err := vol.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := vol.GetBlockExists(ctx, leaf); err != nil || !found {
		t.Fatalf("pinned block after collection = %v, %v; want stored", found, err)
	}
}
