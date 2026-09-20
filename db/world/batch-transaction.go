package world

import "context"

// batchTransaction holds the state of one sequential BatchEngine.Run call.
type batchTransaction struct {
	// tx is opened on the first nested transaction.
	tx Tx
	// aborted records a discarded write.
	aborted bool
	// closed prevents new handles after Run returns.
	closed bool
}

// batchTransactionHandle borrows state without owning its commit or release.
type batchTransactionHandle struct {
	// WorldState supplies the shared batch state.
	WorldState
	// batch owns the physical transaction.
	batch *batchTransaction
	// write distinguishes discarded mutations from completed reads.
	write bool
	// committed records acceptance into the enclosing batch.
	committed bool
}

// Commit accepts this client's writes into the enclosing batch.
func (t *batchTransactionHandle) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.committed = true
	return nil
}

// Discard aborts the enclosing batch when a write was not accepted.
func (t *batchTransactionHandle) Discard() {
	if t.write && !t.committed {
		t.batch.aborted = true
	}
}

// GetObjectBodiesBatch preserves the underlying transport's batched reads.
func (t *batchTransactionHandle) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*ObjectBody, error) {
	return GetObjectBodiesBatch(ctx, t.WorldState, keys)
}

// GetObjectBodiesBatchPage preserves the underlying response byte budget.
func (t *batchTransactionHandle) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, error) {
	return GetObjectBodiesBatchPage(ctx, t.WorldState, keys, byteBudget)
}

var _ Tx = (*batchTransactionHandle)(nil)
