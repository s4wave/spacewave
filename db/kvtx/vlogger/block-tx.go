package kvtx_vlogger

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// BlockTx implements a verbose logger block tx.
type BlockTx struct {
	blockIter atomic.Uint32
	*Tx
	btx kvtx.BlockTx
}

func NewBlockTx(le *logrus.Entry, tx kvtx.BlockTx) *BlockTx {
	return &BlockTx{
		Tx:  NewTx(le, tx),
		btx: tx,
	}
}

// GetCursor returns the block cursor at the root of the tree.
func (t *BlockTx) GetCursor() *block.Cursor {
	return t.btx.GetCursor()
}

// GetCursorAtKey returns the cursor referenced by the key.
//
// Returns nil, nil if not found.
func (t *BlockTx) GetCursorAtKey(ctx context.Context, key []byte) (rbcs *block.Cursor, rerr error) {
	defer func() {
		t.le.Debugf(
			"GetCursorAtKey(%s) => ref(%v) found(%v) err(%v)",
			keyForLogging(key),
			rbcs.GetRef().MarshalLog(),
			rbcs != nil,
			rerr,
		)
	}()
	return t.btx.GetCursorAtKey(ctx, key)
}

// SetCursorAtKey sets the key to a reference to the object at bcs.
// if isBlob is set, the object must be a *blob.Blob (for reading with Get).
// if bcs == nil, the key is set with a empty block ref.
// bcs must not point to a sub-block.
func (t *BlockTx) SetCursorAtKey(ctx context.Context, key []byte, bcs *block.Cursor, isBlob bool) (rerr error) {
	defer func() {
		t.le.Debugf(
			"SetCursorAtKey(%s, %s, %v) => err(%v)",
			keyForLogging(key),
			bcs.GetRef().MarshalLog(),
			isBlob,
			rerr,
		)
	}()
	return t.btx.SetCursorAtKey(ctx, key, bcs, isBlob)
}

// DeleteCursorAtKey deletes the key and returns the cursor to the value.
// returns nil, nil if not found.
func (t *BlockTx) DeleteCursorAtKey(ctx context.Context, key []byte) (rbcs *block.Cursor, rerr error) {
	defer func() {
		t.le.Debugf(
			"DeleteCursorAtKey(%s) => ref(%v) found(%v) err(%v)",
			keyForLogging(key),
			rbcs.GetRef().MarshalLog(),
			rbcs != nil,
			rerr,
		)
	}()
	return t.btx.DeleteCursorAtKey(ctx, key)
}

// BlockIterate returns the block iterator.
func (t *BlockTx) BlockIterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.BlockIterator {
	ii := t.blockIter.Add(1) - 1
	it := t.btx.BlockIterate(ctx, prefix, sort, reverse)
	t.le.Debugf(
		"BlockIterate(%s, %v, %v) => it(%d)",
		keyForLogging(prefix),
		sort, reverse,
		ii,
	)
	le := t.le.WithField("kvtx-vlogger-block-iter-id", ii)
	return NewBlockIterator(le, ii, it)
}

// _ is a type assertion
var (
	_ kvtx.Tx      = ((*Tx)(nil))
	_ kvtx.BlockTx = ((*BlockTx)(nil))
)
