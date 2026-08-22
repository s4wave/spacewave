package store_kvtx_badger

import (
	"context"
	"sync"
	"sync/atomic"

	bdb "github.com/dgraph-io/badger/v4"
	"github.com/s4wave/spacewave/db/kvtx"
)

// gcWriteThreshold is the number of bytes written through the store
// before the next value-log garbage collection runs.
const gcWriteThreshold = 64 << 20

// Store is a badger database key-value store.
type Store struct {
	db       *bdb.DB
	writeMtx sync.Mutex

	// writtenBytes counts bytes written since the last value-log GC.
	writtenBytes atomic.Int64
	// gcSignal is sent when writtenBytes crosses the GC threshold.
	gcSignal chan struct{}
}

// NewStore constructs a new key-value store from a badger db.
func NewStore(db *bdb.DB) *Store {
	return &Store{db: db, gcSignal: make(chan struct{}, 1)}
}

// recordWrite adds n written bytes and signals the GC loop when the
// accumulated total crosses the threshold.
func (s *Store) recordWrite(n int) {
	total := s.writtenBytes.Add(int64(n))
	if total >= gcWriteThreshold && s.writtenBytes.CompareAndSwap(total, 0) {
		select {
		case s.gcSignal <- struct{}{}:
		default:
		}
	}
}

// Open opens a badger database store.
func Open(opts bdb.Options) (*Store, error) {
	b, err := bdb.Open(opts)
	if err != nil {
		return nil, err
	}

	return NewStore(b), nil
}

// GetDB returns the badger DB.
func (s *Store) GetDB() *bdb.DB {
	return s.db
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
//
// Badger allows concurrent writes but returns ErrConflict.
// Our application code is not ErrConflict aware, and in many cases
// expects a single holder for a write transaction at a time.
// For this reason, a write mutex is used.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		s.writeMtx.Lock()
	}
	txn := s.db.NewTransaction(write)
	return s.newTx(txn, write), nil
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
// Execute runs value-log garbage collection when the write path signals
// that enough bytes have been written since the last run.
func (s *Store) Execute(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.gcSignal:
		}
		// Run until a pass reports nothing left to discard, checking the
		// context between passes.
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := s.db.RunValueLogGC(0.5); err != nil {
				break
			}
		}
	}
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
