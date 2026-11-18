package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetObjectBodiesBatchPage reads one page of object bodies and the seqno in
// one transaction.
func (e *EngineTx) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) (bodies []*world.ObjectBody, consumed uint32, seqno uint64, err error) {
	err = e.performOp(ctx, func(tx *Tx) error {
		bodies, consumed, seqno, err = tx.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
		return err
	})
	return bodies, consumed, seqno, err
}
