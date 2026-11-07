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
func (e *EngineTx) GetObjectBodiesBatchPageWithSeqno(
	ctx context.Context,
	keys []string,
	byteBudget int,
) (bodies []*world.ObjectBody, consumed uint32, seqno uint64, err error) {
	// Read one page of object bodies and the transaction seqno.
	err = e.performOp(ctx, func(tx *Tx) error {
		bodies, consumed, seqno, err = tx.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
		return err
	})
	return bodies, consumed, seqno, err
}

// _ is a type assertion
var (
	_ world.ObjectBodyPageBatcher      = (*EngineTx)(nil)
	_ world.ObjectBodyPageSeqnoBatcher = (*EngineTx)(nil)
)
