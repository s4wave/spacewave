//go:build !tinygo

package block_store_writeback

import (
	"sync/atomic"
	"testing"

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
