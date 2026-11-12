//go:build !goscript

package space_world

import (
	"bytes"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// TestAdmitHoldsStoredBlocks checks that a block of a candidate stored past
// the PendingStore's bound outlives every stage that put it, a collection at
// a 1 ns grace, and another writer's put that carries the stages' releases
// into the Volume: Admit holds the candidate whenever a block of it went to
// the Volume, whether its own put stored the block or the writer's stage
// stored it in an early drain while the candidate's own entries fit.
func TestAdmitHoldsStoredBlocks(t *testing.T) {
	for _, tc := range []struct {
		name        string
		writerStage bool
	}{
		{name: "admit-put"},
		{name: "writer-stage", writerStage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			a, collect := newVolumeAdapter(t, 4<<10, time.Nanosecond)
			entry := func(data []byte, refs ...*block.BlockRef) *block.PutBatchEntry {
				t.Helper()
				ref, err := block.BuildBlockRef(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
			}

			// Store a page past the bound, through the writer's stage or the
			// candidate's own entries, under a root that fits.
			page := entry(bytes.Repeat([]byte("page "), 2<<10))
			root := entry([]byte("candidate World"), page.Ref)
			entries := []*block.PutBatchEntry{page, root}
			releaseWriter := func() {}
			if tc.writerStage {
				stage, release, err := a.pages.OpenStage(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := stage.PutBlockBatch(ctx, []*block.PutBatchEntry{page}); err != nil {
					t.Fatal(err)
				}
				entries, releaseWriter = []*block.PutBatchEntry{root}, release
			}

			// Admit the candidate, end the writer's stage, let another
			// writer's put release the stages' roots, and collect.
			adm := a.newAdmission(world.Candidate{Root: root.Ref, Entries: entries}, nil, nil)
			if err := a.storeAdmitted(ctx, adm); err != nil {
				t.Fatal(err)
			}
			releaseWriter()
			if _, _, err := a.pages.StoreOps.PutBlock(ctx, []byte("another writer"), &block.PutOpts{RefsKnown: true}); err != nil {
				t.Fatal(err)
			}
			collect()

			// The page must still read.
			found, err := a.pages.StoreOps.GetBlockExists(ctx, page.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("a collection freed a page of the admitted candidate")
			}
		})
	}
}
