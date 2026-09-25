// Package memtable is an in-memory ordered key-value table with copy-on-write
// snapshots, which its owner persists through a commit function.
//
// Published tables are never changed, so a read transaction sees one snapshot
// for its whole life without locks. A write transaction holds the writer lock
// and changes its own copy, recording each change for the owner to persist.
package memtable

import (
	"bytes"
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Op is one change a write transaction made.
type Op struct {
	// Key is the key.
	Key []byte
	// Value is the value a set stores.
	Value []byte
	// Delete deletes the key.
	Delete bool
}

// CommitFunc persists ops, which turn the published table into next, before
// the table publishes next. With ordered set it may persist them with write
// ordering only, as kvtx.OrderedCommitTx describes. Calls never overlap.
type CommitFunc func(ctx context.Context, next Snapshot, ops []Op, ordered bool) error

// entry is one key and its value.
type entry struct {
	// key is the key.
	key []byte
	// value is the value.
	value []byte
}

// lessEntry orders entries by key.
func lessEntry(a, b entry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// tree is the ordered set of entries. Trees skip the btree's lock: a
// published tree is only read, and a write transaction owns its copy.
type tree = btree.BTreeG[entry]

// Snapshot is one immutable version of the table.
type Snapshot struct {
	// tree holds the entries.
	tree *tree
}

// Len returns the number of keys.
func (s Snapshot) Len() int {
	return s.tree.Len()
}

// Scan calls fn with each key and value in key order until fn returns false.
// The key and value must not be changed.
func (s Snapshot) Scan(fn func(key, value []byte) bool) {
	s.tree.Scan(func(e entry) bool { return fn(e.key, e.value) })
}

// Table is the key-value table.
type Table struct {
	// commit persists each write transaction.
	commit CommitFunc

	// wmtx is held by the open write transaction.
	wmtx sync.Mutex

	// mtx guards tree.
	mtx sync.Mutex
	// tree is the published table.
	tree *tree
}

// New returns an empty table that persists write transactions with commit.
func New(commit CommitFunc) *Table {
	return &Table{
		commit: commit,
		tree:   btree.NewBTreeGOptions(lessEntry, btree.Options{NoLocks: true}),
	}
}

// Load sets key to value in the published table, keeping both slices. It
// serves recovery before the table is shared and is fastest in key order.
func (t *Table) Load(key, value []byte) {
	t.tree.Load(entry{key: key, value: value})
}

// Apply applies ops to the published table. It serves recovery before the
// table is shared.
func (t *Table) Apply(ops []Op) {
	for _, op := range ops {
		if op.Delete {
			t.tree.Delete(entry{key: op.Key})
			continue
		}
		t.tree.Set(entry{key: op.Key, value: op.Value})
	}
}

// NewTransaction opens a transaction on the published table. A write
// transaction holds the table's writer lock until Commit or Discard.
func (t *Table) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		t.mtx.Lock()
		tree := t.tree
		t.mtx.Unlock()
		return &Tx{tree: tree}, nil
	}
	t.wmtx.Lock()
	t.mtx.Lock()
	tree := t.tree.Copy()
	t.mtx.Unlock()
	return &Tx{table: t, tree: tree}, nil
}

// publish persists a write transaction's ops and publishes its tree. The
// caller holds wmtx.
func (t *Table) publish(ctx context.Context, next *tree, ops []Op, ordered bool) error {
	if err := t.commit(ctx, Snapshot{tree: next}, ops, ordered); err != nil {
		return err
	}
	t.mtx.Lock()
	t.tree = next
	t.mtx.Unlock()
	return nil
}

// _ is a type assertion
var _ kvtx.Store = (*Table)(nil)
