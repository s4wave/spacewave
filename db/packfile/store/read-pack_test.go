package store

import (
	"bytes"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// TestReadPackBlocks verifies a whole-pack read returns every block in value
// order with one transport fetch.
func TestReadPackBlocks(t *testing.T) {
	ordered := []struct{ Name, Data string }{
		{"c", "charlie"},
		{"a", "alpha"},
		{"b", "bravo"},
	}
	packBytes, _ := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())

	blocks, err := store.ReadPackBlocks(t.Context(), "pack", int64(len(packBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != len(ordered) {
		t.Fatalf("read %d blocks, want %d", len(blocks), len(ordered))
	}
	for i, b := range blocks {
		if string(b.Block.GetData()) != ordered[i].Data {
			t.Fatalf("block %d = %q, want %q", i, b.Block.GetData(), ordered[i].Data)
		}
		if err := block.NewBlockRef(b.Hash).VerifyData(b.Block.GetData(), false); err != nil {
			t.Fatalf("block %d hash: %v", i, err)
		}
	}
	transport.mtx.Lock()
	calls := len(transport.calls)
	transport.mtx.Unlock()
	if calls != 1 {
		t.Fatalf("fetched %d times, want 1", calls)
	}
}

// TestReadPackBlocksRejectsCorruptedBlock verifies a whole-pack read fails on
// a value that does not match its key hash.
func TestReadPackBlocksRejectsCorruptedBlock(t *testing.T) {
	packBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, transport := openerFromBytes(packBytes)
	transport.rewriteFn = func(_ int, _ int64, data []byte) []byte {
		return bytes.ReplaceAll(data, []byte("alpha"), []byte("alphx"))
	}
	store := NewPackfileStore(opener, newMemIndexCache())

	_, err := store.ReadPackBlocks(t.Context(), "pack", int64(len(packBytes)))
	if !errors.Is(err, block.ErrBlockRefMismatch) {
		t.Fatalf("err=%v, want ErrBlockRefMismatch", err)
	}
}
