package kvtx

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/tx"
)

// BlockTxOps contains extra tx ops for a block-backed store.
type BlockTxOps interface {
	// SetCursorAsRef sets a cursor as a cid.BlockRef in the tree.
	// If bcs != nil, adds a reference from the BlockRef to bcs.
	// This sets the value of key to a reference to the object at bcs.
	// Returns the block cursor located at the node containing key.
	SetCursorAsRef(key []byte, bcs *block.Cursor) (*block.BlockRef, *block.Cursor, error)
	// BlockIterate returns the block iterator.
	BlockIterate(prefix []byte, sort, reverse bool) BlockIterator
	// GetWithCursor returns the value of the specified key, if it exists, and a
	// block cursor located at the value sub-block.
	//
	// Returns nil, nil, nil if not found.
	GetWithCursor(key []byte) ([]byte, *block.Cursor, error)
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
