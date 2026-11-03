package store_kvtx_inmem

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Store is an in-memory transactional key-value store.
// A writer's FIFO turn excludes other writers through Commit or Discard.
// Its admission waits for existing readers and blocks later readers; no
// broadcast lock is held while waiting for a turn or for readers to drain.
type Store struct {
	// tree holds committed values under transaction admission.
	tree *btree.BTreeG[*valType]

	// writers hands each writer a FIFO turn held until Commit or Discard.
	writers csync.Mutex

	// bcast guards the admission fields below.
	bcast broadcast.Broadcast
	// nreaders counts admitted read transactions.
	nreaders int
	// writing reports an admitted write transaction.
	writing bool
	// writeWaiting counts queued writers and the turn holder draining readers.
	// It excludes an admitted writer; each caller unregisters once on admission
	// or cancellation, keeping later readers behind waiting writers.
	writeWaiting int
}

// NewStore constructs a new key-value store.
func NewStore() *Store {
	return &Store{tree: btree.NewBTreeG[*valType](valTypeLess)}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// A writer owns a FIFO turn before it waits for readers to drain.
	if write {
		return s.newWriteTransaction(ctx)
	}

	// Readers overlap while no writer holds or is waiting for admission.
	var tx kvtx.Tx
	err := s.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		// A waiting writer retains priority over later readers.
		if s.writing || s.writeWaiting != 0 {
			return false, nil
		}

		// Admit another reader to the shared committed tree.
		s.nreaders++
		tx = newTx(s, false, nil)
		return true, nil
	})
	return tx, err
}

// newWriteTransaction retains its FIFO turn through publication or discard.
// Registrations block new readers while earlier readers drain; cancellation
// drops exactly this caller's registration and any turn it already acquired.
func (s *Store) newWriteTransaction(ctx context.Context) (kvtx.Tx, error) {
	// Register before entering the turnstile so fresh readers cannot bypass us.
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.writeWaiting++
	})
	registered := true
	var releaseTurn func()
	defer func() {
		// A failed admission owns no transaction to release these resources.
		if registered {
			s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				s.writeWaiting--
				broadcast()
			})
		}
		if releaseTurn != nil {
			releaseTurn()
		}
	}()

	// Existing queued writers receive their turns before this caller can retry.
	var err error
	releaseTurn, err = s.writers.Lock(ctx)
	if err != nil {
		return nil, err
	}

	// Only the turn holder waits on the reader drain, then transfers its turn.
	var tx kvtx.Tx
	err = s.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		// Keep the FIFO turn while the earlier readers finish.
		if s.nreaders != 0 || s.writing {
			return false, nil
		}

		// Transfer registration and turn ownership into the admitted transaction.
		s.writeWaiting--
		registered = false
		s.writing = true
		tx = newTx(s, true, releaseTurn)
		releaseTurn = nil
		return true, nil
	})
	return tx, err
}

// WritersWaiting reports queued writers, including the turn draining readers.
func (s *Store) WritersWaiting() bool {
	var waiting bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		waiting = s.writeWaiting != 0
	})
	return waiting
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// SupportsAtomicCommit advertises atomic, synchronous in-process transactions,
// not process-persistent durability.
func (s *Store) SupportsAtomicCommit() bool { return true }

// _ is a type assertion
var (
	_ kvtx.Store            = (*Store)(nil)
	_ kvtx.WriterQueueStore = (*Store)(nil)
)
