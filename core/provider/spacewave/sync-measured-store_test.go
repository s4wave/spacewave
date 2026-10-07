package provider_spacewave

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// syncMeasuredStore counts pending-marker work on a real transactional backend.
type syncMeasuredStore struct {
	// store supplies the transactional storage contract.
	store kvtx.Store
	// commitErr injects a write failure before the backend durability barrier.
	commitErr error
	// visits counts marker values examined by scans, iterators, and point reads.
	visits atomic.Int64
	// page records the largest read transaction's candidate count.
	page atomic.Int64
	// reading counts read transactions that are still open.
	reading atomic.Int64
}

// NewTransaction retains the backend's commit and discard behavior.
func (s *syncMeasuredStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write {
		s.reading.Add(1)
	}
	return &syncMeasuredTx{tx: tx, store: s, write: write}, nil
}

// _ is a type assertion.
var _ kvtx.Store = (*syncMeasuredStore)(nil)
