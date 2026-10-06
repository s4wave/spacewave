//go:build darwin || linux

package s4db

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// tentry is one change buffered in a write transaction.
type tentry struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// data is the new value.
	data []byte
}

// lessTEntry orders transaction entries by key.
func lessTEntry(a, b tentry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// newChanges returns an empty change buffer.
func newChanges() *btree.BTreeG[tentry] {
	return btree.NewBTreeGOptions(lessTEntry, btree.Options{NoLocks: true})
}

// Tx is a transaction on one snapshot. A write transaction buffers its
// changes and holds the writer lock.
type Tx struct {
	// db is the database.
	db *DB
	// st is the snapshot.
	st *state
	// stripe is the counter of st holding the snapshot.
	stripe int
	// changes buffers the writes of a write transaction; nil when read-only.
	changes *btree.BTreeG[tentry]
	// done is set after Commit or Discard.
	done bool
}

// Size returns the number of keys.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	// A finished transaction has no snapshot.
	if t.done {
		return 0, kvtx.ErrDiscarded
	}

	// Adjust the snapshot's count by each buffered change that adds or
	// removes a key.
	n := t.st.count
	var err error
	if t.changes != nil {
		t.changes.Scan(func(c tentry) bool {
			var found bool
			_, found, err = t.db.lookup(t.st, c.key)
			switch {
			case err != nil:
				return false
			case found && c.del:
				n--
			case !found && !c.del:
				n++
			}
			return true
		})
	}
	return uint64(max(n, 0)), err
}

// Get returns the value of key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Check the transaction and key.
	switch {
	case t.done:
		return nil, false, kvtx.ErrDiscarded
	case len(key) == 0:
		return nil, false, kvtx.ErrEmptyKey
	}

	// A buffered change decides the key.
	if t.changes != nil {
		if c, ok := t.changes.Get(tentry{key: key}); ok {
			return c.data, !c.del, nil
		}
	}

	// Otherwise read the snapshot.
	v, found, err := t.db.lookup(t.st, key)
	if err != nil || !found {
		return nil, false, err
	}
	data, err := t.db.readValue(v)
	return data, err == nil, err
}

// Exists reports whether key is present.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	switch {
	case t.done:
		return false, kvtx.ErrDiscarded
	case len(key) == 0:
		return false, kvtx.ErrEmptyKey
	}
	if t.changes != nil {
		if c, ok := t.changes.Get(tentry{key: key}); ok {
			return !c.del, nil
		}
	}
	_, found, err := t.db.lookup(t.st, key)
	return found, err
}

// Set buffers a write of key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if err := t.writable(key); err != nil {
		return err
	}
	t.changes.Set(tentry{key: bytes.Clone(key), data: bytes.Clone(value)})
	return nil
}

// Delete buffers a deletion of key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if err := t.writable(key); err != nil {
		return err
	}
	t.changes.Set(tentry{key: bytes.Clone(key), del: true})
	return nil
}

// writable checks that the transaction may write key.
func (t *Tx) writable(key []byte) error {
	switch {
	case t.done:
		return kvtx.ErrDiscarded
	case t.changes == nil:
		return kvtx.ErrNotWrite
	case len(key) == 0:
		return kvtx.ErrEmptyKey
	case len(key) > MaxKeySize:
		return errors.Errorf("key of %d bytes exceeds %d", len(key), MaxKeySize)
	}
	return nil
}

// ScanPrefix calls cb with each key and value under prefix in key order.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		v, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(it.Key(), v); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefixKeys calls cb with each key under prefix in key order.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// Iterate returns an iterator over keys under prefix, always in key order.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	it := newIterator(t.db, t.st, t.changes, prefix, reverse)
	if t.done {
		it.err = kvtx.ErrDiscarded
	}
	return it
}

// Commit writes the changes with a full flush.
func (t *Tx) Commit(ctx context.Context) error {
	return t.commit(false)
}

// CommitOrdered writes the changes ordered after earlier commits without
// waiting for the drive to persist them.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	return t.commit(true)
}

// commit finishes the transaction, writing its changes.
func (t *Tx) commit(ordered bool) error {
	// A read-only transaction has nothing to write.
	if t.done {
		return kvtx.ErrDiscarded
	}
	if t.changes == nil {
		t.Discard()
		return nil
	}

	// Collect the changes in key order.
	t.done = true
	db := t.db
	changes := make([]tentry, 0, t.changes.Len())
	t.changes.Scan(func(c tentry) bool {
		changes = append(changes, c)
		return true
	})

	// Write them, then run checkpoint and compaction work with the snapshot
	// released, and release the writer lock.
	err := db.commit(t.st, changes, ordered)
	db.release(t.st, t.stripe)
	if err == nil {
		err = db.afterCommit()
	}
	seq := db.cur.Load().seq
	db.unlockWriter()
	if err != nil || ordered {
		return err
	}

	// Flush outside the writer lock, sharing the flush with concurrent
	// commits.
	return db.syncThrough(seq, 0)
}

// Discard ends the transaction without writing.
func (t *Tx) Discard() {
	if t.done {
		return
	}
	t.done = true
	t.db.release(t.st, t.stripe)
	if t.changes != nil {
		t.db.unlockWriter()
	}
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)
