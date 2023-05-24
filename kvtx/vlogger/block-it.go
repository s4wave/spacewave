package kvtx_vlogger

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
)

// BlockIterator implements the block iterator verbose logger.
type BlockIterator struct {
	*Iterator
	blk kvtx.BlockIterator
}

// ValueCursor returns a cursor located at the "value" sub-block.
// Returns nil if the iterator is not at a valid location.
func (b *BlockIterator) ValueCursor() (rbcs *block.Cursor) {
	defer func() {
		err := b.blk.Err()
		b.le.Debugf(
			"ValueCursor() => ref(%v) found(%v) err(%v)",
			rbcs.GetRef().MarshalLog(),
			rbcs != nil,
			err,
		)
	}()
	return b.blk.ValueCursor()
}

// _ is a type assertion
var _ kvtx.BlockIterator = ((*BlockIterator)(nil))
