package engine

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// NewTransaction opens a lazy generation-consistent transaction.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Reject cancellation and use after Close.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.mtx.Lock()
	closed := e.closed
	e.mtx.Unlock()
	if closed {
		return nil, ErrClosed
	}

	// Number the transaction and record its opening.
	t := &transaction{engine: e, id: e.workloadIDs.Add(1), write: write, pending: make(map[string]*Record)}
	op := workload.OpTxRead
	if write {
		op = workload.OpTxWrite
	}
	workload.Record{Op: op, ID: t.id}.Log(ctx)
	return t, nil
}

// Execute satisfies the durable volume store lifecycle; writes are synchronous.
func (e *Engine) Execute(ctx context.Context) error {
	return nil
}

// _ verifies the public transactional store contract.
var _ kvtx.Store = (*Engine)(nil)
