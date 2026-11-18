package world

import "context"

// GetObjectBodiesBatchPage reads one page of object bodies and its seqno in
// one read transaction.
func (e *engineWorldState) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) (bodies []*ObjectBody, consumed uint32, seqno uint64, err error) {
	err = e.performOp(ctx, false, func(tx Tx) error {
		bodies, consumed, seqno, err = tx.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
		return err
	})
	return bodies, consumed, seqno, err
}
