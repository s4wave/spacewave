package block_gc

import (
	"context"
	"encoding/binary"

	"github.com/s4wave/spacewave/db/kvtx"
)

// CoalesceSharingBasisChanges makes generation changes share one advance in
// this graph's physical transaction. Call only on a graph over one transaction,
// before its first write; discard the graph with that transaction. Its caller
// serializes mutations, as it does CountOwnerEdges and SettleOwnerCounts.
func (g *RefGraph) CoalesceSharingBasisChanges() {
	if g.basisChanged == nil {
		g.basisChanged = new(bool)
	}
}

// invalidateSharingBasisTx invalidates receipts whose funded incarnation may
// have disappeared. Rebuilds and erasing a tracked block's count metadata use
// the same generation; ordinary count changes and cache-byte eviction do not.
// Erasure requests a recount; an already-running walk keeps its cursor. A physical
// transaction publishes at most one advance, including mixed erasure/recount.
func (g *RefGraph) invalidateSharingBasisTx(ctx context.Context, tx kvtx.TxOps) error {
	// A complete physical transaction exposes only its final basis to receipts.
	if g.basisChanged != nil && *g.basisChanged {
		return nil
	}
	generation, err := readSharingGenerationTx(ctx, tx)
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, sharingGenerationKey, binary.AppendUvarint(nil, generation+1)); err != nil {
		return err
	}
	if g.basisChanged != nil {
		*g.basisChanged = true
	}
	return nil
}
