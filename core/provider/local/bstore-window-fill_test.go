package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// heldSet is a local store that holds a set of blocks.
type heldSet struct {
	block.StoreOps
	held map[string]bool
}

func (h *heldSet) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		out[i] = h.held[ref.MarshalString()]
	}
	return out, nil
}

// TestWindowFillSkipsHeld checks that a window fill fills each block the local
// store lacks, once, and skips the blocks it holds.
func TestWindowFillSkipsHeld(t *testing.T) {
	var refs []*block.BlockRef
	var entries []*block.PutBatchEntry
	for _, s := range []string{"a", "b", "c"} {
		ref, err := block.BuildBlockRef([]byte(s), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: []byte(s)})
	}
	local := &heldSet{held: map[string]bool{refs[1].MarshalString(): true}}
	var filled []string
	ops := &windowFillOps{StoreOps: local, fill: func(_ context.Context, primary block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
		if primary != local {
			t.Fatal("fill got another primary")
		}
		filled = append(filled, string(stored.Data))
		return nil
	}}
	have, err := ops.PutBlockBatch(t.Context(), entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(filled) != 2 || filled[0] != "a" || filled[1] != "c" {
		t.Fatalf("filled %v, want a and c", filled)
	}
	if have[0] || !have[1] || have[2] {
		t.Fatalf("have = %v", have)
	}
}
