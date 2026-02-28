package block_gc

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
)

// putMockBlock stores a mock Example block and returns its ref.
func putMockBlock(t *testing.T, ctx context.Context, store block.StoreOps, msg string) *block.BlockRef {
	t.Helper()
	ex := block_mock.NewExample(msg)
	ref, _, err := block.PutBlock(ctx, store, ex)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// TestExtractBlockRefs_Nil tests that nil returns nil.
func TestExtractBlockRefs_Nil(t *testing.T) {
	refs := block.ExtractBlockRefs(nil)
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs from nil, got %d", len(refs))
	}
}

// TestExtractBlockRefs_NoRefs tests a block with no refs.
func TestExtractBlockRefs_NoRefs(t *testing.T) {
	ex := block_mock.NewExample("hello")
	refs := block.ExtractBlockRefs(ex)
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs from Example, got %d", len(refs))
	}
}

// TestExtractBlockRefs_DirectRefs tests a block with direct refs.
func TestExtractBlockRefs_DirectRefs(t *testing.T) {
	ctx := context.Background()
	mockStore := block_mock.NewMockStore(0)

	target := putMockBlock(t, ctx, mockStore, "target")
	sub := &block_mock.SubBlock{ExamplePtr: target}
	refs := block.ExtractBlockRefs(sub)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref from SubBlock, got %d", len(refs))
	}
	if !refs[0].EqualsRef(target) {
		t.Fatalf("expected ref to target, got %s", refs[0].MarshalString())
	}
}

// TestExtractBlockRefs_SubBlockRefs tests recursive extraction through
// BlockWithSubBlocks -> BlockWithRefs.
func TestExtractBlockRefs_SubBlockRefs(t *testing.T) {
	ctx := context.Background()
	mockStore := block_mock.NewMockStore(0)

	target := putMockBlock(t, ctx, mockStore, "nested-target")
	root := &block_mock.Root{
		ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: target},
	}

	refs := block.ExtractBlockRefs(root)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref from Root with SubBlock, got %d", len(refs))
	}
	if !refs[0].EqualsRef(target) {
		t.Fatalf("expected ref to nested target, got %s", refs[0].MarshalString())
	}
}

// TestExtractBlockRefs_EmptySubBlock tests that empty sub-blocks return no refs.
func TestExtractBlockRefs_EmptySubBlock(t *testing.T) {
	root := &block_mock.Root{
		ExampleSubBlock: &block_mock.SubBlock{},
	}
	refs := block.ExtractBlockRefs(root)
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs from Root with empty SubBlock, got %d", len(refs))
	}
}

// TestExtractBlockRefs_NilSubBlockRef tests that nil refs in sub-blocks are skipped.
func TestExtractBlockRefs_NilSubBlockRef(t *testing.T) {
	root := &block_mock.Root{
		ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: nil},
	}
	refs := block.ExtractBlockRefs(root)
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs from SubBlock with nil ptr, got %d", len(refs))
	}
}
