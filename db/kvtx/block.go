package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
)

// BlockTxOps contains extra tx ops for a block-backed store.
type BlockTxOps interface {
	// GetCursor returns the block cursor at the root of the tree.
	GetCursor() *block.Cursor
	// GetCursorAtKey returns the cursor referenced by the key.
	//
	// Returns nil, nil if not found.
	GetCursorAtKey(ctx context.Context, key []byte) (*block.Cursor, error)
	// SetCursorAtKey sets the key to a reference to the object at bcs.
	// if isBlob is set, the object must be a *blob.Blob (for reading with Get).
	// if bcs == nil, the key is set with a empty block ref.
	// bcs must not point to a sub-block.
	SetCursorAtKey(ctx context.Context, key []byte, bcs *block.Cursor, isBlob bool) error
	// DeleteCursorAtKey deletes the key and returns the cursor to the value.
	// returns nil, nil if not found.
	DeleteCursorAtKey(ctx context.Context, key []byte) (*block.Cursor, error)
	// BlockIterate returns the block iterator.
	BlockIterate(ctx context.Context, prefix []byte, sort, reverse bool) BlockIterator
}

// CastBlockTxOps casts a TxOps to a BlockTxOps or returns ErrBlockTxOpsUnimplemented.
func CastBlockTxOps(ops TxOps) (BlockTxOps, error) {
	if ops == nil {
		return nil, nil
	}
	tops, ok := ops.(BlockTxOps)
	if !ok {
		return nil, ErrBlockTxOpsUnimplemented
	}
	return tops, nil
}

// BlockIterator is a kvtx iterator backed by a block graph.
type BlockIterator interface {
	// Iterator is the kvtx iterator interface.
	Iterator
	// ValueCursor returns a cursor located at the "value" sub-block.
	// Returns nil if the iterator is not at a valid location.
	ValueCursor() *block.Cursor
}

// BlockTx is a database transaction backed by a block graph.
// Concurrent calls are not safe on a single transaction.
type BlockTx interface {
	// TxOps contains the transaction operations.
	TxOps
	// BlockTxOps contains the block graph transaction operations.
	BlockTxOps

	// Tx contains the transaction confirm.
	tx.Tx
}

// CastBlockTx casts a Tx to a BlockTx or returns ErrBlockTxOpsUnimplemented.
func CastBlockTx(tx Tx) (BlockTx, error) {
	if tx == nil {
		return nil, nil
	}
	tops, ok := tx.(BlockTx)
	if !ok {
		return nil, ErrBlockTxOpsUnimplemented
	}
	return tops, nil
}
