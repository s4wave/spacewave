package block_store_writeback

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackedStore writes a tree and loose blocks through a Store that
// diverts new values to a packahead Log: the Log holds the values and the
// Volume their child lists without the bytes or any marker; reads
// come back with their refs through every step of the read chain; a block the
// Volume held before and a cache fill stay in the Volume; with the switch off
// writes go to the Volume and mark again while the packed blocks still read;
// and after a restart every block still reads.
func TestPackedStore(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	dir := t.TempDir()
	log, err := packahead.Open(ctx, dir, packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	open := func(divert bool) *Store {
		s, err := NewStoreWithPacks(ctx, NewPackedStore(log, ownedStore{env.vol}, divert), env.vol, env.markers(t), testMarkersID, true)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open(true)

	// Land a two-block tree and a loose block.
	leaf := []byte("packed leaf")
	leafRef, _ := block.BuildBlockRef(leaf, nil)
	parent := []byte("packed parent")
	parentRef, _ := block.BuildBlockRef(parent, nil)
	_, err = s.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: leafRef, Data: leaf, RefsKnown: true},
		{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef}, RefsKnown: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	loose, _, err := s.PutBlock(ctx, []byte("packed loose"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	packed := []*block.BlockRef{leafRef, parentRef, loose}

	// The Log holds the values; the Volume holds no bytes and no marker.
	for _, ref := range packed {
		if !log.Has(packfile.BlockKey(ref.GetHash())) {
			t.Fatalf("block %s not in the Log", ref.MarshalString())
		}
		if _, found, err := env.vol.GetBlock(ctx, ref); err != nil || found {
			t.Fatalf("Volume holds the bytes of %s: %v %v", ref.MarshalString(), found, err)
		}
		if env.isMarked(t, ref) {
			t.Fatalf("diverted block %s has a marker", ref.MarshalString())
		}
	}

	// Every block reads back with its refs, and exists.
	checkRead := func(s *Store, ref *block.BlockRef, data []byte, refs int) {
		t.Helper()
		stored, err := s.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !bytes.Equal(stored.Data, data) || len(stored.Refs) != refs || !stored.RefsKnown {
			t.Fatalf("read %s: %v, %v", ref.MarshalString(), stored, err)
		}
		if found, err := s.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("exists %s: %v, %v", ref.MarshalString(), found, err)
		}
	}
	checkRead(s, leafRef, leaf, 0)
	checkRead(s, parentRef, parent, 1)
	checkRead(s, loose, []byte("packed loose"), 0)

	// A diverted publication commits its head and its child lists without the
	// bytes, and a sweep keeps the block its bucket holds.
	pub := NewPackedStore(log, ownedStore{env.vol}, true)
	pubLeaf := []byte("published leaf")
	pubLeafRef, _ := block.BuildBlockRef(pubLeaf, nil)
	pubParent := []byte("published parent")
	pubParentRef, _ := block.BuildBlockRef(pubParent, nil)
	err = pub.PublishAtomic(ctx, &block.AtomicPublication{
		Entries: []*block.PutBatchEntry{
			{Ref: pubLeafRef, Data: pubLeaf, RefsKnown: true},
			{Ref: pubParentRef, Data: pubParent, Refs: []*block.BlockRef{pubLeafRef}, RefsKnown: true},
		},
		BucketID: "bucket",
		TrackGC:  true,
		Heads: []*block.AtomicHeadUpdate{{ObjectStoreID: "head", Key: []byte("head"), Replace: func(context.Context, []byte, bool) ([]byte, error) {
			return []byte("published"), nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*block.BlockRef{pubLeafRef, pubParentRef} {
		if _, found, err := env.vol.GetBlock(ctx, ref); err != nil || found {
			t.Fatalf("Volume holds the bytes of published %s: %v %v", ref.MarshalString(), found, err)
		}
	}
	rg := env.vol.GetRefGraph()
	if refs, err := rg.GetOutgoingRefs(ctx, block_gc.BlockIRI(pubParentRef)); err != nil || len(refs) != 1 || refs[0] != block_gc.BlockIRI(pubLeafRef) {
		t.Fatalf("published parent's child list = %v, %v", refs, err)
	}
	swept, err := env.vol.SweepUnreferenced(ctx, rg, []string{block_gc.BlockIRI(pubParentRef)})
	if err != nil || len(swept) != 0 {
		t.Fatalf("sweep of the held published parent removed %v, %v", swept, err)
	}
	checkRead(s, pubLeafRef, pubLeaf, 0)
	checkRead(s, pubParentRef, pubParent, 1)

	// A repeated write reports the block stored and appends nothing.
	before := log.GetStats().LocalBytes
	existed, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: leafRef, Data: leaf, RefsKnown: true}})
	if err != nil || !existed[0] || log.GetStats().LocalBytes != before {
		t.Fatalf("repeated write: existed %v err %v, %d to %d bytes", existed, err, before, log.GetStats().LocalBytes)
	}

	// A block the Volume held before reports stored, and a cache fill stays in
	// the Volume alone.
	old := []byte("volume block")
	oldRef, _ := block.BuildBlockRef(old, nil)
	if _, err := env.vol.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: oldRef, Data: old, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	existed, err = s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: oldRef, Data: old, RefsKnown: true}})
	if err != nil || !existed[0] {
		t.Fatalf("write of a Volume block: existed %v err %v", existed, err)
	}
	fill, _, err := s.PutBlock(ctx, []byte("filled from the remote"), &block.PutOpts{RefsKnown: true, CacheFill: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := env.vol.GetBlock(ctx, fill); !found || log.Has(packfile.BlockKey(fill.GetHash())) {
		t.Fatalf("cache fill: in Volume %v, in Log %v", found, log.Has(packfile.BlockKey(fill.GetHash())))
	}
	checkRead(s, oldRef, old, 0)

	// With the switch off, a write goes to the Volume and marks, and the
	// packed blocks still read.
	off := open(false)
	plain, _, err := off.PutBlock(ctx, []byte("written with the switch off"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := env.vol.GetBlock(ctx, plain); !found || !env.isMarked(t, plain) {
		t.Fatalf("switched-off write: in Volume %v, marked %v", found, env.isMarked(t, plain))
	}
	checkRead(off, parentRef, parent, 1)

	// After a restart of the Volume and the Log, every block reads.
	log.Close()
	env.open(t)
	log, err = packahead.Open(ctx, dir, packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	s = open(true)
	checkRead(s, leafRef, leaf, 0)
	checkRead(s, parentRef, parent, 1)
	checkRead(s, loose, []byte("packed loose"), 0)
	checkRead(s, pubParentRef, pubParent, 1)
	checkRead(s, plain, []byte("written with the switch off"), 0)
}
