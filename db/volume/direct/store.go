// Package direct is a volume engine that stores every key-value entry, block,
// and journal entry as its own record in a records.Store, the shape of an
// IndexedDB object store.
//
// The key-value store and the journal also live in a memtable loaded at Open,
// so reads never reach the record store. Each write transaction commits as one
// record store call carrying its changes and the block writes and removes made
// since the previous commit. Record store calls apply atomically and in order,
// so an ordered commit may publish blocks too: a crash that keeps it keeps
// every call before it.
package direct

import (
	"context"
	"encoding/binary"
	"maps"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
	"github.com/s4wave/spacewave/db/volume/memtable"
	"github.com/s4wave/spacewave/db/volume/records"
)

// Record key prefixes.
const (
	// blockPrefix holds block payloads by reference key.
	blockPrefix = "b/"
	// journalPrefix holds garbage collection journal entries by sequence.
	journalPrefix = "j/"
	// kvPrefix holds the key-value store.
	kvPrefix = "k/"
)

// Store is a volume engine on a record store.
type Store struct {
	// kvtx.Store serves the key-value store under kvPrefix.
	kvtx.Store

	// records holds every record.
	records records.Store
	// table holds the key-value store and journal records.
	table *memtable.Table
	// unflushed is set while an ordered commit is not yet durable. The
	// table's writer lock guards it.
	unflushed bool
	// journal is the sequence of the last journal entry. The table's writer
	// lock guards it.
	journal uint64

	// mtx guards the fields below.
	mtx sync.Mutex
	// pending holds block writes and removes not yet committed, by record
	// key.
	pending map[string]*pendingBlock
}

// pendingBlock is one block write or remove not yet committed.
type pendingBlock struct {
	// data is the payload; nil for a remove.
	data []byte
}

// Open opens the store on rs, loading the key-value store and the journal.
func Open(ctx context.Context, rs records.Store) (*Store, error) {
	s := &Store{records: rs, pending: make(map[string]*pendingBlock)}
	s.table = memtable.New(s.commit)
	s.Store = kvtx_prefixer.NewPrefixer(tableStore{s}, []byte(kvPrefix))

	// Load the journal and key-value records in key order.
	for _, prefix := range []string{journalPrefix, kvPrefix} {
		err := rs.Scan(ctx, []byte(prefix), func(key, value []byte) error {
			s.table.Load(key, value)
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "load records")
		}
	}

	// Find the last journal sequence.
	err := s.view(ctx, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte(journalPrefix), true, true)
		defer it.Close()
		if it.Next() {
			s.journal = binary.BigEndian.Uint64(it.Key()[len(journalPrefix):])
		}
		return it.Err()
	})
	if err != nil {
		return nil, errors.Wrap(err, "read journal")
	}
	return s, nil
}

// Close releases the store. Uncommitted block writes are lost.
func (s *Store) Close() error {
	return nil
}

// view runs fn in a table read transaction.
func (s *Store) view(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.table.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// update runs fn in a table write transaction and commits it, durably unless
// ordered is set.
func (s *Store) update(ctx context.Context, ordered bool, fn func(tx kvtx.Tx) error) error {
	tx, err := s.table.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := fn(tx); err != nil {
		return err
	}
	if ordered {
		return kvtx.CommitOrdered(ctx, tx)
	}
	return tx.Commit(ctx)
}

// commit is the table's commit function. It writes ops and the pending
// blocks in one record store call, durable unless ordered is set, and drops
// the committed entries from pending; entries replaced meanwhile stay. With
// nothing to write it makes the earlier ordered commits durable unless ordered
// is set.
func (s *Store) commit(ctx context.Context, _ memtable.Snapshot, ops []memtable.Op, ordered bool) error {
	// Snapshot the pending blocks.
	s.mtx.Lock()
	published := maps.Clone(s.pending)
	s.mtx.Unlock()

	// Write the changes and the blocks in one call.
	recs := make([]records.Op, 0, len(ops)+len(published))
	for _, op := range ops {
		recs = append(recs, records.Op{Key: op.Key, Value: op.Value, Delete: op.Delete})
	}
	for _, key := range slices.Sorted(maps.Keys(published)) {
		data := published[key].data
		recs = append(recs, records.Op{Key: []byte(key), Value: data, Delete: data == nil})
	}
	if len(recs) == 0 && (ordered || !s.unflushed) {
		return nil
	}
	if err := s.records.Commit(ctx, recs, !ordered); err != nil {
		return err
	}
	s.unflushed = ordered

	// Drop the entries the call committed.
	s.mtx.Lock()
	for key, p := range published {
		if s.pending[key] == p {
			delete(s.pending, key)
		}
	}
	s.mtx.Unlock()
	return nil
}

// tableStore serves table transactions. A write transaction collects its
// changes and applies them in one table write transaction at commit, so any
// number of write transactions can be open at once.
type tableStore struct {
	s *Store
}

// NewTransaction opens a table transaction.
func (t tableStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	read, err := t.s.table.NewTransaction(ctx, false)
	if err != nil || !write {
		return read, err
	}
	w := &writeTx{s: t.s, ctx: ctx}
	w.Tx, err = kvtx_txcache.NewTxWithCbs(read, true, read.Discard, w.begin, false)
	if err != nil {
		read.Discard()
		return nil, err
	}
	return w, nil
}

// writeTx is a buffered table write transaction.
type writeTx struct {
	// Tx collects the changes.
	*kvtx_txcache.Tx
	// s is the store.
	s *Store
	// ctx is the context the transaction was opened with.
	ctx context.Context
	// ttx is the table write transaction opened at commit.
	ttx kvtx.Tx
}

// begin opens the table write transaction the collected changes apply to.
func (w *writeTx) begin() (kvtx.Tx, error) {
	ttx, err := w.s.table.NewTransaction(w.ctx, true)
	if err != nil {
		return nil, err
	}
	w.ttx = ttx
	return ttx, nil
}

// Commit applies the collected changes and commits them durably with the
// pending blocks.
func (w *writeTx) Commit(ctx context.Context) error {
	return w.commit(ctx, false)
}

// CommitOrdered applies the collected changes and commits them with the
// pending blocks and write ordering only.
func (w *writeTx) CommitOrdered(ctx context.Context) error {
	return w.commit(ctx, true)
}

// commit applies the collected changes and commits the table transaction.
func (w *writeTx) commit(ctx context.Context, ordered bool) error {
	err := w.Tx.Commit(ctx)
	if w.ttx == nil {
		return err
	}
	defer w.ttx.Discard()
	if err != nil {
		return err
	}
	if ordered {
		return kvtx.CommitOrdered(ctx, w.ttx)
	}
	return w.ttx.Commit(ctx)
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*writeTx)(nil)
