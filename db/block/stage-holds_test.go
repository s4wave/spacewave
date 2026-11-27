package block_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// TestStageWrappersHoldCompleteRoots checks holds through opened and wrapped stages.
func TestStageWrappersHoldCompleteRoots(t *testing.T) {
	// Build the root each wrapper must forward unchanged.
	ref, err := block.BuildBlockRef([]byte("held root"), &block.PutOpts{HashType: hash.HashType_HashType_SHA256})
	if err != nil {
		t.Fatal(err)
	}

	// Cover stages opened through wrappers and wrappers added above a stage.
	for name, wrap := range map[string]func(block.StoreOps) block.StoreOps{
		"read-only":  func(inner block.StoreOps) block.StoreOps { return block.NewReadOnlyStore(inner) },
		"read-write": func(inner block.StoreOps) block.StoreOps { return block.NewStoreRW(nil, inner) },
		"buffered":   func(inner block.StoreOps) block.StoreOps { return block.NewBufferedStore(t.Context(), inner) },
	} {
		t.Run(name, func(t *testing.T) {
			// Open a stage through the wrapper first.
			block_store_test.RequireStageHold(t, wrap)

			// A wrapper around an existing stage must preserve its holds too.
			inner := &block_store_test.HoldingStage{}
			held, err := block.HoldCompleteRoots(t.Context(), wrap(inner), []*block.BlockRef{ref})
			if err != nil {
				t.Fatal(err)
			}
			inner.RequireHold(t, held, ref)
		})
	}
}

// TestBufferedStoreHoldDoesNotFlushPendingWrites keeps holds separate from publication.
func TestBufferedStoreHoldDoesNotFlushPendingWrites(t *testing.T) {
	// Buffer an unrelated block above a stage that already proves a root.
	ctx := t.Context()
	physical := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0)
	inner := &block_store_test.HoldingStage{StoreOps: physical}
	buffered := block.NewBufferedStore(ctx, inner)
	pending, _, err := buffered.PutBlock(ctx, []byte("pending write"), nil)
	if err != nil {
		t.Fatal(err)
	}
	proved, _, err := physical.PutBlock(ctx, []byte("proved root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Holding the proved root must reach the stage without draining the buffer.
	held, err := block.HoldCompleteRoots(ctx, buffered, []*block.BlockRef{proved})
	if err != nil {
		t.Fatal(err)
	}
	inner.RequireHold(t, held, proved)
	if exists, err := physical.GetBlockExists(ctx, pending); err != nil || exists {
		t.Fatalf("pending write reached inner store during hold: exists=%v err=%v", exists, err)
	}

	// Explicit Sync still drains the pending block after the hold.
	if _, err := buffered.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if exists, err := physical.GetBlockExists(ctx, pending); err != nil || !exists {
		t.Fatalf("pending write missing after sync: exists=%v err=%v", exists, err)
	}
}
