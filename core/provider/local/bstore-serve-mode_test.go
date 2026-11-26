package provider_local

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestServeModeMountReadsBackendFillsNothing checks that a replacement mount
// can read evicted blocks from the backend without filling the local volume.
func TestServeModeMountReadsBackendFillsNothing(t *testing.T) {
	// Bound both the remount and the backend reads.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Place a store on the fake backend and lose its local copies.
	p := setupPlacedStore(ctx, t)
	prev := p.loseLocalBlocks(ctx, t)

	// Remount the store in serve mode.
	SetServeOnly(true)
	defer SetServeOnly(false)
	p.acc.bstores.RestartRoutine(p.tkr.id)
	bs, err := p.tkr.bstoreCtr.WaitValueWithValidator(ctx, func(bs *BlockStore) (bool, error) {
		return bs != nil && bs != prev, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bs.placement.serveOnly {
		t.Fatal("store remounted without serve mode")
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
