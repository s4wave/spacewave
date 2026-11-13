package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
)

// Store is a transactional key/value store.
type Store interface {
	// NewTransaction returns a new transaction against the store.
	// Always call Discard() after you are done with the transaction.
	// The transaction will be read-only unless write is set.
	NewTransaction(ctx context.Context, write bool) (Tx, error)
}

// CoordinationRefreshStore refreshes a store at an external coordination
// boundary before callers open transactions derived from that boundary.
type CoordinationRefreshStore interface {
	// RefreshForCoordinationLock reloads the store's view after the caller
	// takes the external coordination lock.
	RefreshForCoordinationLock() error
}

// WatchEntry is one key/value pair in a watched store snapshot.
type WatchEntry struct {
	// Key is the entry key.
	Key []byte
	// Value is the entry value.
	Value []byte
}

// WatchStore streams key/value snapshots after committed store changes.
type WatchStore interface {
	// WatchPrefix calls cb with the current prefix snapshot and each changed snapshot.
	WatchPrefix(ctx context.Context, prefix []byte, cb func(entries []WatchEntry) error) error
}

// TxOps contains the database transaction operations.
type TxOps interface {
	// Size returns the number of keys in the store.
	Size(ctx context.Context) (uint64, error)
	// Get returns values for a key.
	Get(ctx context.Context, key []byte) (data []byte, found bool, err error)
	// Exists checks if a key exists.
	Exists(ctx context.Context, key []byte) (bool, error)
	// Set sets the value of a key.
	// This will not be committed until Commit is called.
	Set(ctx context.Context, key, value []byte) error
	// Delete deletes a key.
	// This will not be committed until Commit is called.
	// Not found should not return an error.
	Delete(ctx context.Context, key []byte) error
	// ScanPrefix iterates over keys with a prefix.
	//
	// Note: neither key nor value should be retained outside cb() without
	// copying.
	//
	// Note: the ordering of the scan is not necessarily sorted.
	ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error
	// ScanPrefixKeys iterates over keys only with a prefix.
	ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error
	// Iterate returns an iterator with a given key prefix.
	//
	// Should always return non-nil, with error field filled if necessary.
	// If sort, iterates in sorted order, reverse reverses the key iteration.
	// The prefix is NOT clipped from the output keys.
	// If !sort, reverse MAY have no effect.
	// Must call Next() or Seek() before valid.
	// Some implementations return BlockIterator.
	// Context is used for the iterator and internally for iterator operations.
	// Return an ErrorIterator if anything goes wrong building the iterator.
	Iterate(ctx context.Context, prefix []byte, sort, reverse bool) Iterator
}

// Iterator iterates over a kvtx Tx store with a given prefix.
// Note: Next() or Seek() must be called before iterator is valid.
type Iterator interface {
	// Err returns any error that has closed the iterator.
	// May return context.Canceled or ErrDiscarded if closed.
	Err() error
	// Valid returns if the iterator points to a valid entry.
	//
	// If err is set, returns false.
	Valid() bool
	// Key returns the current entry key, or nil if not valid.
	//
	// NOTE: even if prefix is set this does not trim the prefix.
	Key() []byte
	// Value returns the current entry value, or nil if not valid.
	//
	// May cache the value between calls, copy if modifying.
	Value() ([]byte, error)
	// ValueCopy copies the value to the given byte slice and returns it.
	// If the slice is not big enough (cap), it must create a new one and return it.
	// May use the value cached from Value() call as the source of the data.
	// May return nil if !Valid().
	ValueCopy([]byte) ([]byte, error)
	// Next advances to the next entry and returns Valid.
	Next() bool
	// Seek moves the iterator to the first key >= k, or <= k in reverse mode.
	//
	// Pass nil to seek to the beginning, or the end if reversed. Seek positions
	// the iterator, so Next is not needed after it. If prefix is set, k should
	// have the prefix or be nil; Seek does not prepend it. Seek fails in one of
	// three ways: it returns an error without modifying the iterator, it sets
	// the iterator Err and returns nil, or it sets Err and returns the error.
	Seek(k []byte) error
	// Close closes the iterator.
	// Note: it is not necessary to close all iterators before Discard().
	Close()
}

// Tx is a database transaction.
// Concurrent calls are not safe on a single transaction.
type Tx interface {
	// TxOps contains the transaction operations.
	TxOps

	// Tx contains the transaction confirm.
	tx.Tx
}

// OrderedCommitTx is a transaction that can commit with write ordering in place
// of a full durability flush. CommitOrdered finishes the transaction like Commit,
// but a crash may lose it along with later commits, never an earlier one. The
// next Commit makes it durable.
type OrderedCommitTx interface {
	Tx
	// CommitOrdered commits the transaction with write ordering.
	CommitOrdered(ctx context.Context) error
}

// CommitOrdered commits tx with write ordering when it implements
// OrderedCommitTx, and with a full Commit otherwise.
func CommitOrdered(ctx context.Context, tx Tx) error {
	if otx, ok := tx.(OrderedCommitTx); ok {
		return otx.CommitOrdered(ctx)
	}
	return tx.Commit(ctx)
}

// WithOrderedCommit returns tx with Commit replaced by CommitOrdered, for
// callers that commit through a generic transaction runner.
func WithOrderedCommit(tx Tx) Tx {
	return orderedCommitTx{Tx: tx}
}

// orderedCommitTx commits its transaction with write ordering.
type orderedCommitTx struct {
	Tx
}

// Commit commits the transaction with write ordering when supported.
func (t orderedCommitTx) Commit(ctx context.Context) error {
	return CommitOrdered(ctx, t.Tx)
}

// OrderedCommitStore is a store whose write transactions implement
// OrderedCommitTx. WaitDurable waits for durability without forcing a flush.
type OrderedCommitStore interface {
	Store
	// Sync makes every completed ordered commit durable.
	Sync(ctx context.Context) error
	// WaitDurable waits until every completed ordered commit is durable.
	WaitDurable(ctx context.Context) error
}

// AtomicCommitStore opts into using one physical transaction as a publication
// fence. Commit must synchronously finish the transaction atomically, with the
// durability barrier the store is configured for, and Discard must roll back
// every mutation. The fence is atomicity: a store configured not to sync still
// commits all of a transaction or none of it. A write transaction's reads must
// see the writes of every committed transaction of every process, so a check
// and the write it guards land together. Wrappers that
// defer Commit across calls, and stores that split blocks into another domain,
// must not advertise this capability. In-memory stores can implement it for
// transactional tests; this does not make their data process-persistent.
type AtomicCommitStore interface {
	Store
	// SupportsAtomicCommit reports whether the store provides the fence.
	SupportsAtomicCommit() bool
}

// WriterQueueStore is a store whose write transactions exclude each other and
// that reports when writers queue. A write transaction doing deferrable work,
// such as garbage collection, commits early when one waits. It admits a
// waiting writer before a write transaction that asks later, by arrival order
// or by holding a WithBackgroundWriteContext transaction behind the others,
// so background work that ends a transaction and begins the next queues
// behind the writer that waited.
type WriterQueueStore interface {
	Store
	// WritersWaiting reports whether a write transaction waits to begin.
	WritersWaiting() bool
}

// ForegroundQueueStore is a WriterQueueStore with a prioritized writer turn
// that reports foreground writers apart. The store cannot shorten a
// transaction it has admitted, so the owner of a background transaction, one
// opened with WithBackgroundWriteContext, keeps the turn short: it does its
// work in bounded steps, checks ForegroundWaiting between them, and commits
// at the first step boundary after a foreground writer waits, continuing in a
// new transaction that queues behind it. A step boundary must leave the
// store consistent if the process stops there. A waiting background writer
// is not a reason to commit early: it gains nothing from the hand-off, since
// background work finishes later rather than sooner either way, while every
// early commit costs one.
type ForegroundQueueStore interface {
	WriterQueueStore
	// ForegroundWaiting reports whether a foreground write transaction waits
	// to begin.
	ForegroundWaiting() bool
}

// ForegroundWaiting reports whether a foreground writer waits on store. On a
// store without a prioritized turn every writer counts, since each waits on
// the holder the same way; a store without a writer queue reports none.
func ForegroundWaiting(store Store) bool {
	if q, ok := store.(ForegroundQueueStore); ok {
		return q.ForegroundWaiting()
	}
	q, ok := store.(WriterQueueStore)
	return ok && q.WritersWaiting()
}
