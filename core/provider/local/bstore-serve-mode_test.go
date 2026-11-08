package provider_local

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestServeModeMountWritesNothing remounts a placed store in serve mode after
// its local blocks are gone: the backend, which serve mode opens for reads
// alone, serves both blocks, and neither fills the local volume.
func TestServeModeMountReadsBackendFillsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Place a store on the fake backend and lose its local copies.
	p := setupPlacedStore(ctx, t)
	prev := p.loseLocalBlocks(ctx, t)

	// Remount the store in serve mode.
	SetServeOnly(true)
	defer SetServeOnly(false)
	p.acc.bstores.RestartRoutine(p.tkr.id)
	var bs *BlockStore
	for {
		var err error
		bs, err = p.tkr.bstoreCtr.WaitValue(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if bs != prev && bs.placement.serveOnly {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("store never remounted in serve mode")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Read both blocks from the backend, filling nothing.
	for ref, want := range p.blocks {
		data, found, err := bs.GetBlock(ctx, ref)
		if err != nil || !found || !bytes.Equal(data, want) {
			t.Fatalf("serve read %s = %q/%v/%v", ref.MarshalString(), data, found, err)
		}
	}
	held, err := p.acc.GetVolume().GetBlockExistsBatch(ctx, []*block.BlockRef{p.rootRef, p.childRef})
	if err != nil {
		t.Fatal(err)
	}
	if held[0] || held[1] {
		t.Fatalf("serve reads filled the local volume: %v", held)
	}
}
