package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetObjectBodiesBatchPage returns one budgeted page from one transaction.
func (e *EngineTx) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, error) {
	bodies, consumed, _, err := e.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	return bodies, consumed, err
}

// GetObjectBodiesBatchPageWithSeqno returns one budgeted page and its transaction seqno.
func (e *EngineTx) GetObjectBodiesBatchPageWithSeqno(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, uint64, error) {
	var bodies []*world.ObjectBody
	var consumed uint32
	var seqno uint64
	err := e.performOp(ctx, func(tx *Tx) error {
		var err error
		bodies, consumed, seqno, err = tx.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
		return err
	})
	return bodies, consumed, seqno, err
}

var _ world.ObjectBodyPageBatcher = ((*EngineTx)(nil))
var _ world.ObjectBodyPageSeqnoBatcher = ((*EngineTx)(nil))
