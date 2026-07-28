package world

import "context"

// GetObjectBodiesBatch returns object bodies from one read transaction.
func (e *engineWorldState) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*ObjectBody, error) {
	var bodies []*ObjectBody
	err := e.performOp(ctx, false, func(tx Tx) error {
		var berr error
		bodies, berr = GetObjectBodiesBatch(ctx, tx, keys)
		return berr
	})
	return bodies, err
}

// GetObjectBodiesBatchPage returns one encoded-size-bounded page from one read
// transaction.
func (e *engineWorldState) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*ObjectBody, uint32, error) {
	bodies, consumed, _, err := e.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	return bodies, consumed, err
}

// GetObjectBodiesBatchPageWithSeqno returns one page and the sequence number
// observed by the read transaction.
func (e *engineWorldState) GetObjectBodiesBatchPageWithSeqno(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*ObjectBody, uint32, uint64, error) {
	var bodies []*ObjectBody
	var consumed uint32
	var seqno uint64
	err := e.performOp(ctx, false, func(tx Tx) error {
		var berr error
		seqno, berr = tx.GetSeqno(ctx)
		if berr != nil {
			return berr
		}
		bodies, consumed, berr = GetObjectBodiesBatchPage(ctx, tx, keys, byteBudget)
		return berr
	})
	return bodies, consumed, seqno, err
}
