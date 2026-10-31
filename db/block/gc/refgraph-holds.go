package block_gc

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// AddHolds adds one shared count to each block in blocks, once per entry, in
// one transaction. A block's shared count is its holders minus one: a holder
// is a page or root that names it. It adds nothing while the counts are not
// complete, since the sharing walk counts the same holders from the graph.
func (g *RefGraph) AddHolds(ctx context.Context, blocks []string) error {
	if len(blocks) == 0 {
		return nil
	}
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		s, err := readSharingTx(ctx, tx)
		if err != nil || !s.Phase.Counting() {
			return err
		}
		for _, block := range blocks {
			if !isBlockNode(block) {
				return errors.Errorf("hold on %q, which is not a block", block)
			}
			n, err := readHoldsTx(ctx, tx, block)
			if err != nil {
				return err
			}
			if err := tx.Set(ctx, graphKey(kindHold, block), binary.AppendUvarint(nil, n+1)); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetHolds returns the shared count of block, or 0 when it has none.
func (g *RefGraph) GetHolds(ctx context.Context, block string) (uint64, error) {
	var n uint64
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		n, err = readHoldsTx(ctx, tx, block)
		return err
	})
	return n, err
}

// readHoldsTx reads the shared count of block, or 0 when it has none.
func readHoldsTx(ctx context.Context, tx kvtx.TxOps, block string) (uint64, error) {
	// Read the record, which is absent at zero.
	value, found, err := tx.Get(ctx, graphKey(kindHold, block))
	if err != nil || !found {
		return 0, err
	}

	// Decode the whole value as one uvarint.
	n, size := binary.Uvarint(value)
	if size <= 0 || size != len(value) {
		return 0, errors.Errorf("shared count of %s is malformed", block)
	}
	return n, nil
}
