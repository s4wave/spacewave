package block_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestSetRefMovesStoredBlock reuses the stored ref of a clean block moved
// under a new parent instead of encoding and writing it again.
func TestSetRefMovesStoredBlock(t *testing.T) {
	// Store root -> leaf.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	tx, root := block.NewTransaction(store, nil, nil, nil)
	root.SetBlock(&block_mock.SubBlock{}, true)
	root.FollowRef(1, nil).SetBlock(block_mock.NewExample("leaf"), true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Load the stored leaf in a new transaction.
	tx, root = block.NewTransaction(store, nil, rootRef, nil)
	rootBlk, err := root.Unmarshal(ctx, block_mock.NewSubBlockBlock)
	if err != nil {
		t.Fatal(err)
	}
	leaf := root.FollowRef(1, rootBlk.(*block_mock.SubBlock).GetExamplePtr())
	if _, err := block_mock.UnmarshalExample(ctx, leaf); err != nil {
		t.Fatal(err)
	}

	// Move the leaf under a new middle block.
	mid := root.Detach(false)
	mid.ClearAllRefs()
	mid.SetBlock(&block_mock.SubBlock{}, true)
	mid.SetRef(1, leaf)
	root.SetRef(1, mid)

	// Only the middle block and the root change.
	writeCtx, counter := block.WithWriteCounter(ctx)
	rootRef, _, err = tx.Write(writeCtx, true)
	if err != nil {
		t.Fatal(err)
	}
	if writes := counter.Snapshot().BlockWriteCount; writes != 2 {
		t.Fatalf("block writes = %d, want 2", writes)
	}

	// Read back the middle block.
	_, root = block.NewTransaction(store, nil, rootRef, nil)
	rootBlk, err = root.Unmarshal(ctx, block_mock.NewSubBlockBlock)
	if err != nil {
		t.Fatal(err)
	}
	mid = root.FollowRef(1, rootBlk.(*block_mock.SubBlock).GetExamplePtr())
	midBlk, err := mid.Unmarshal(ctx, block_mock.NewSubBlockBlock)
	if err != nil {
		t.Fatal(err)
	}

	// The middle block must reference the stored leaf.
	leaf = mid.FollowRef(1, midBlk.(*block_mock.SubBlock).GetExamplePtr())
	body, err := block_mock.UnmarshalExample(ctx, leaf)
	if err != nil || body.GetMsg() != "leaf" {
		t.Fatalf("moved leaf readback: %v, %v", body, err)
	}
}
