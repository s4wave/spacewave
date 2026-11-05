package sobject_world_engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// untrackedRetainer is a store that owns roots but holds block bytes without
// their refs.
type untrackedRetainer struct {
	block.StoreOps
	block.RootRetainer
}

// SupportsRootRetention reports that the store owns roots.
func (untrackedRetainer) SupportsRootRetention() bool { return true }

// TestRootRejectsUnknownRefs checks that holding an untracked root fails
// naming the block, instead of storing it as a leaf.
func TestRootRejectsUnknownRefs(t *testing.T) {
	// Hold a root that the store has without its refs.
	ctx := t.Context()
	inner := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	root, _, err := inner.PutBlock(ctx, []byte("untracked root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// A hold and a retained checkpoint each fail naming the root.
	hold := newRootHold(&untrackedRetainer{StoreOps: inner})
	holdErr := hold.world(ctx, "world", root)
	if len(hold.entries) != 0 || len(hold.roots) != 0 {
		t.Fatalf("hold gathered %d entries and %d roots", len(hold.entries), len(hold.roots))
	}
	_, storeErr := storeRootBlock(ctx, &testSharedObject{blockStore: newTestBlockStore("untracked", inner)}, root)
	for name, err := range map[string]error{"hold": holdErr, "store": storeErr} {
		if !errors.Is(err, block.ErrRefsUnknown) || !strings.Contains(err.Error(), root.MarshalString()) {
			t.Fatalf("%s = %v, want ErrRefsUnknown naming %s", name, err, root.MarshalString())
		}
	}
}
