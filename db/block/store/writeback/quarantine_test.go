//go:build !tinygo

package block_store_writeback

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// TestUploadReputsQuarantinedBlock uploads a block again through the
// write-back when only a quarantined pack holds it: the locate skips the
// quarantined pack, so the upload writes the block to a new registered pack,
// while a block a registered pack holds is skipped unread.
func TestUploadReputsQuarantinedBlock(t *testing.T) {
	ctx := t.Context()
	_, client := s3test.New(t)
	remote := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, "p/", &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()),
		SoleWriter: true,
	})
	defer remote.Close()
	packIDs := func() map[string]bool {
		t.Helper()
		packs, err := remote.Packs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids := make(map[string]bool, len(packs))
		for _, entry := range packs {
			ids[entry.GetId()] = true
		}
		return ids
	}

	// The remote holds each block in its own pack; quarantine the second.
	if _, _, err := remote.PutBlock(ctx, []byte("kept"), leafOpts); err != nil {
		t.Fatal(err)
	}
	before := packIDs()
	if _, _, err := remote.PutBlock(ctx, []byte("quarantined"), leafOpts); err != nil {
		t.Fatal(err)
	}
	var held string
	for id := range packIDs() {
		if !before[id] {
			held = id
		}
	}
	if held == "" {
		t.Fatal("the second block shares the first block's pack")
	}
	if err := remote.Quarantine(ctx, []string{held}); err != nil {
		t.Fatal(err)
	}

	// The write-back holds both blocks again, with the locate switch on.
	env := newTestEnv(t)
	s := env.newStore(t, true)
	var on atomic.Bool
	on.Store(true)
	s.SetLocateUploaded(&on)
	var refs []*block.BlockRef
	for _, data := range []string{"kept", "quarantined"} {
		ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	found, err := remote.LocateBlocks(ctx, refs)
	if err != nil || !found[0] || found[1] {
		t.Fatalf("located %v, %v; want only the kept block", found, err)
	}

	// The drain skips the kept block and writes the quarantined one to a new
	// pack, which places it.
	before = packIDs()
	drain(t, s, remote)
	after := packIDs()
	if len(after) != len(before)+1 {
		t.Fatalf("the drain wrote %d packs, want 1", len(after)-len(before))
	}
	if found, err = remote.LocateBlocks(ctx, refs); err != nil || !found[0] || !found[1] {
		t.Fatalf("located %v, %v after the drain; want both", found, err)
	}
	if env.isMarked(t, refs[0]) || env.isMarked(t, refs[1]) {
		t.Fatal("a marker stayed after the drain")
	}
	if data, ok, err := remote.GetBlock(ctx, refs[1]); err != nil || !ok || string(data) != "quarantined" {
		t.Fatalf("read the re-put block = %q, %v, %v", data, ok, err)
	}
	if !after[held] {
		t.Fatal("the quarantined pack left the listing")
	}
}

// TestEvictPassHoldsSweepRecheck opens an eviction pass, verifies a pack
// registered, then quarantines the pack before the pass drops anything, as a
// sweep can between a pass's verify and its drops. The sweep trashes the pack
// only after a recheck that waits while the store's oldest open read began no
// later than the pack's hold time (packsweep's WaitOpenRead), so the pass must
// show as that read until it ends; a later verify finds the pack unregistered.
// It fails if openEvictPass records nothing.
func TestEvictPassHoldsSweepRecheck(t *testing.T) {
	// One remote pack holds the block.
	ctx := t.Context()
	_, client := s3test.New(t)
	remote := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, "p/", &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()),
		SoleWriter: true,
	})
	defer remote.Close()
	if _, _, err := remote.PutBlock(ctx, []byte("verified"), leafOpts); err != nil {
		t.Fatal(err)
	}
	packs, err := remote.Packs(ctx)
	if err != nil || len(packs) != 1 {
		t.Fatalf("%d packs, %v", len(packs), err)
	}
	id := packs[0].GetId()
	s := newTestEnv(t).newStore(t, true)
	if ops := s.OldestOperations(); !ops.Read.IsZero() {
		t.Fatalf("an idle store has a read open since %v", ops.Read)
	}

	// A pass opens and verifies the pack, then the sweep holds it.
	end := s.openEvictPass()
	if !remote.Registered(id) {
		t.Fatal("the pass cannot verify a registered pack")
	}
	if err := remote.Quarantine(ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	holds, err := remote.Holds(ctx, []string{id})
	if err != nil || len(holds) != 1 || holds[0].State != block_store_s3.PackQuarantined {
		t.Fatalf("holds = %+v, %v", holds, err)
	}
	since := time.Unix(0, holds[0].Since)

	// The open pass holds back the recheck, and verifies nothing more.
	if ops := s.OldestOperations(); ops.Read.IsZero() || ops.Read.After(since) {
		t.Fatalf("the open pass shows read %v, which does not hold back a pack held at %v", ops.Read, since)
	}
	if remote.Registered(id) {
		t.Fatal("a quarantined pack still verifies")
	}

	// Ended, it holds nothing back.
	end()
	if ops := s.OldestOperations(); !ops.Read.IsZero() {
		t.Fatalf("an ended pass left a read open since %v", ops.Read)
	}
}
