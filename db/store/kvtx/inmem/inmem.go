package store_kvtx_inmem

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Store is an in-memory transactional key-value store.
//
// A read transaction reads the tree published when it opened and never waits
// for a writer. One write transaction runs at a time, holding the writer turn
// from open to Commit or Discard: a transaction opened with
// block.WithBackground waits behind every waiting foreground writer, and each
// priority goes in arrival order. A commit publishes a copy-on-write copy of
// the tree, so a published tree never changes under its readers. A read
// transaction that is never discarded pins its tree version, and every node
// later commits copied away from it, in memory.
type Store struct {
	// writers grants the writer turn.
	writers csync.PrioMutex
	// waiting counts the write transactions queued on writers.
	waiting atomic.Int32

	// mtx guards tree.
	mtx sync.Mutex
	// tree is the published tree of committed values.
	tree *btree.BTreeG[*valType]
}

// Priorities of Store.writers.
const (
	// writerBackground is the priority of write transactions opened with
	// block.WithBackground.
	writerBackground = iota - 1
	// writerForeground is the priority of every other write transaction.
	writerForeground
)

// NewStore constructs a new key-value store.
func NewStore() *Store {
	return &Store{tree: btree.NewBTreeG[*valType](valTypeLess)}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// A reader takes the published tree at once.
	if !write {
		return newTx(s, s.published(), false, nil), nil
	}

	// A writer queues for the turn at its priority, counted for
	// WritersWaiting, and reads the tree the last writer published.
	prio := writerForeground
	if block.IsBackground(ctx) {
		prio = writerBackground
	}
	s.waiting.Add(1)
	release, err := s.writers.Lock(ctx, prio)
	s.waiting.Add(-1)
	if err != nil {
		return nil, err
	}
	return newTx(s, s.published(), true, release), nil
}

// published returns the published tree.
func (s *Store) published() *btree.BTreeG[*valType] {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.tree
}

// publish makes tree the published tree. The caller holds the writer turn.
func (s *Store) publish(tree *btree.BTreeG[*valType]) {
	s.mtx.Lock()
	s.tree = tree
	s.mtx.Unlock()
}

// WritersWaiting reports whether a write transaction waits for the writer
// turn.
func (s *Store) WritersWaiting() bool {
	return s.waiting.Load() != 0
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
