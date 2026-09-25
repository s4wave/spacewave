package logindex

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Tx is a transaction on one table snapshot. A write transaction changes its
// own copy of the table and records each change for the log.
type Tx struct {
	// index is the index a write transaction commits to, nil for a read.
	index *Index
	// tree is the snapshot, or the write transaction's copy.
	tree *table
	// ops records the write transaction's changes in order.
	ops []logOp
	// done is set once the transaction commits or is discarded.
	done bool
}

// Size returns the number of keys.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	if t.done {
		return 0, kvtx.ErrDiscarded
	}
	return uint64(t.tree.Len()), nil //nolint:gosec
}

// Get returns the value of key. The value must not be changed.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if t.done {
		return nil, false, kvtx.ErrDiscarded
	}
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	e, ok := t.tree.Get(entry{key: key})
	return e.value, ok, nil
}

// Exists reports whether key is set.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	_, ok, err := t.Get(ctx, key)
	return ok, err
}

// Set sets key to a copy of value.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if err := t.writable(key); err != nil {
		return err
	}
	op := logOp{key: bytes.Clone(key), value: bytes.Clone(value)}
	t.tree.Set(entry{key: op.key, value: op.value})
	t.ops = append(t.ops, op)
	return nil
}

// Delete deletes key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if err := t.writable(key); err != nil {
		return err
	}
	if _, ok := t.tree.Delete(entry{key: key}); ok {
		t.ops = append(t.ops, logOp{key: bytes.Clone(key), del: true})
	}
	return nil
}

// writable checks that the transaction takes a write of key.
func (t *Tx) writable(key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if t.done {
		return kvtx.ErrDiscarded
	}
	if t.index == nil {
		return kvtx.ErrNotWrite
	}
	return nil
}

// ScanPrefix calls cb with each key with prefix and its value in key order.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	if t.done {
		return kvtx.ErrDiscarded
	}
	var err error
	t.tree.Ascend(entry{key: prefix}, func(e entry) bool {
		if !bytes.HasPrefix(e.key, prefix) {
			return false
		}
		err = cb(e.key, e.value)
		return err == nil
	})
	return err
}

// ScanPrefixKeys calls cb with each key with prefix in key order.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, _ []byte) error { return cb(key) })
}

// Iterate returns a sorted iterator over the keys with prefix.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	if t.done {
		return kvtx.NewErrIterator(kvtx.ErrDiscarded)
	}
	return newIterator(t.tree, prefix, reverse)
}

// Commit makes a write transaction's changes durable and publishes them. A
// write transaction without changes writes nothing.
func (t *Tx) Commit(ctx context.Context) error {
	return t.commit(ctx, true)
}

// CommitOrdered publishes a write transaction's changes and appends them to
// the log without a flush. A crash may lose them along with every later
// commit; the next Commit or device flush makes them durable.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	return t.commit(ctx, false)
}

// commit ends the transaction and commits its changes, durably if flush is
// set.
func (t *Tx) commit(ctx context.Context, flush bool) error {
	if t.done {
		return kvtx.ErrDiscarded
	}
	defer t.Discard()
	if t.index == nil || len(t.ops) == 0 {
		return nil
	}
	return t.index.commit(ctx, t.tree, t.ops, flush)
}

// Discard ends the transaction, dropping uncommitted changes.
func (t *Tx) Discard() {
	if t.done {
		return
	}
	t.done = true
	if t.index != nil {
		t.index.wmtx.Unlock()
	}
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)
