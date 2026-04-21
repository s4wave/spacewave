package kvtx_vlogger

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// BlockIterator implements the block iterator verbose logger.
type BlockIterator struct {
	*Iterator
	blk kvtx.BlockIterator
}

// NewBlockIterator constructs a new BlockIterator.
func NewBlockIterator(le *logrus.Entry, ii uint32, it kvtx.BlockIterator) *BlockIterator {
	return &BlockIterator{
		Iterator: NewIterator(le, ii, it),
		blk:      it,
	}
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
