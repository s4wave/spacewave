package kvtx_block_okra

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// inlineValueLimit is the size limit for keeping small metadata values with
// their page. Larger values retain the existing Blob block and chunking path.
const inlineValueLimit = 256

// NewTxWithInlineValues opens an Okra transaction whose owning store is tagged
// KV_IMPL_TYPE_OKRA_INLINE. Existing external value refs remain readable.
func NewTxWithInlineValues(
	ctx context.Context,
	bcs *block.Cursor,
	btx *block.Transaction,
	write bool,
	rootChangedCb func(*block.Cursor),
) (*Tx, error) {
	t, err := NewTx(ctx, bcs, btx, write, rootChangedCb)
	if err != nil {
		return nil, err
	}
	t.inlineValues = true
	return t, nil
}

// DefaultLevelsPerBlock is the levels per block a new tree tagged
// KV_IMPL_TYPE_OKRA_MULTI_LEVEL records.
const DefaultLevelsPerBlock = 2

// NewTxWithLevels opens an Okra transaction with inline values whose owning
// store is tagged KV_IMPL_TYPE_OKRA_MULTI_LEVEL. A tree records its levels
// per block in its root; an empty tree takes levelsPerBlock. One level per
// block records zero, as a tree written before the count existed does.
func NewTxWithLevels(
	ctx context.Context,
	bcs *block.Cursor,
	btx *block.Transaction,
	write bool,
	rootChangedCb func(*block.Cursor),
	levelsPerBlock uint32,
) (*Tx, error) {
	t, err := NewTxWithInlineValues(ctx, bcs, btx, write, rootChangedCb)
	if err != nil {
		return nil, err
	}
	if t.root.GetSize() == 0 && levelsPerBlock > 1 {
		t.levelsPerBlock = levelsPerBlock
	}
	return t, nil
}

// buildValueEntry chooses the value's storage layout. An inline value borrows
// the caller's bytes. The tree builder copies them before the next entry.
func (t *Tx) buildValueEntry(ctx context.Context, key, value []byte) (BuildEntry, error) {
	// Store small values inline when the transaction allows inline values.
	entry := BuildEntry{Key: key, ValueIsBlob: true}
	if t.inlineValues && len(value) <= inlineValueLimit {
		entry.ValueBlob = blob.NewRawBlob(value)
		return entry, ctx.Err()
	}

	// Otherwise write the value as a blob and reference it.
	ref, err := t.buildBlobValue(ctx, value)
	entry.ValueRef = ref
	return entry, err
}

// hashBuildEntry separates inline Blob content from either external ref form.
func hashBuildEntry(entry BuildEntry) ([]byte, error) {
	if entry.ValueBlob == nil {
		return hashLeaf(entry.Key, entry.ValueRef, entry.ValueIsBlob)
	}
	data, err := entry.ValueBlob.MarshalVT()
	if err != nil {
		return nil, err
	}
	return hashKeyValue(entry.Key, append([]byte{2}, data...))
}
