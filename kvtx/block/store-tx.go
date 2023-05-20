package kvtx_block

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/tx"
)

// storeTx is a kvtx transaction for a Store.
type storeTx struct {
	// BlockTx is the underlying block tx.
	kvtx.BlockTx
	// rel indicates if this tx is released
	rel uint32
	// st is the store
	st *Store
	// writeTx is the write transaction.
	// if nil, this is a read-only transaction
	writeTx kvtx.BlockTx
	// writeBtx is the write block transaction.
	// nil if writeTx is nil
	writeBtx *block.Transaction
}

// newStoreTx constructs a new store tx.
// writxTx and btx are nil if read-only
func (s *Store) newStoreTx(writeTx kvtx.BlockTx, btx *block.Transaction) *storeTx {
	tx := writeTx
	if tx == nil {
		tx = s.readTx
	}
	return &storeTx{
		BlockTx:  tx,
		st:       s,
		writeTx:  writeTx,
		writeBtx: btx,
	}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (s *storeTx) Commit(ctx context.Context) error {
	if s.writeTx == nil {
		s.Discard()
		return tx.ErrNotWrite
	}

	// ensure tx is not already discarded
	// also marks the tx as discarded
	if !s.release() {
		return tx.ErrDiscarded
	}

	// commit underlying write tx
	commitErr := s.writeTx.Commit(ctx)

	// commit block transaction
	var nroot *block.BlockRef
	if commitErr == nil {
		nroot, _, commitErr = s.writeBtx.Write(true)
		if commitErr == nil {
			commitErr = nroot.Validate()
		}
	}

	// apply committed changes or rollback
	s.st.rmtx.Lock()
	if s.st.writeTx != s {
		// discarded mid-write
		if commitErr == nil {
			commitErr = tx.ErrDiscarded
		}
	} else {
		s.st.writeTx = nil // clear write tx
		// call commitFn if set
		if commitErr == nil {
			nextRootRef := s.st.root.GetRef().Clone()
			nextRootRef.RootRef = nroot
			// call the commit function if set
			if s.st.commitFn != nil {
				commitErr = s.st.commitFn(nextRootRef.Clone())
			}
			if commitErr == nil {
				commitErr = s.st.setRootRefLocked(ctx, nextRootRef)
			}
		}
	}
	s.st.rmtx.Unlock()
	s.st.wmtx.Release(1)

	return commitErr
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (s *storeTx) Discard() {
	if s.release() {
		if s.writeTx != nil {
			s.writeTx.Discard()
			s.st.wmtx.Release(1)
		}
	}
}

// release releases the tx
func (s *storeTx) release() bool {
	rel := atomic.SwapUint32(&s.rel, 1)
	return rel != 1
}

// _ is a type assertion
var _ kvtx.BlockTx = ((*storeTx)(nil))
