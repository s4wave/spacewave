package s4db

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Tx is a transaction on one snapshot. A write transaction buffers its
// changes and holds the writer lock, or, opened optimistically, records what
// it reads and takes the lock only at Commit.
type Tx struct {
	// db is the database.
	db *DB
	// st is the snapshot.
	st *state
	// stripe is the counter of st holding the snapshot.
	stripe int
	// changes buffers the writes of a write transaction; nil when read-only.
	changes *btree.BTreeG[tentry]
	// reads records the snapshot reads of an optimistic write transaction;
	// nil otherwise.
	reads *readSet
	// atCommit holds the writes an optimistic transaction deferred to its
	// commit, in order.
	atCommit []func(context.Context, kvtx.Tx) error
	// done is set after Commit or Discard.
	done bool
}

// Size returns the number of keys.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	// A finished transaction has no snapshot.
	if t.done {
		return 0, kvtx.ErrDiscarded
	}
	if t.reads != nil {
		t.reads.all = true
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

// Get returns the value of key. The caller owns the returned bytes.
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
			return bytes.Clone(c.data), !c.del, nil
		}
	}

	// Otherwise read the snapshot. An extent is read into a new buffer, and
	// an inline value is copied out of the shared index page.
	t.read(key)
	v, found, err := t.db.lookup(t.st, key)
	if err != nil || !found {
		return nil, false, err
	}
	data, err := t.db.p.readValue(v)
	if err != nil {
		return nil, false, err
	}
	if !v.isRef {
		data = bytes.Clone(data)
	}
	return data, true, nil
}

// GetBatch returns owned values of keys in caller order. Buffered changes
// answer their keys, and the snapshot resolves the rest together, so a cold
// batch waits for one round of reads per tree level instead of one read per
// key.
func (t *Tx) GetBatch(ctx context.Context, keys [][]byte) ([][]byte, []bool, error) {
	// Check the transaction and keys.
	if t.done {
		return nil, nil, kvtx.ErrDiscarded
	}
	for _, key := range keys {
		if len(key) == 0 {
			return nil, nil, kvtx.ErrEmptyKey
		}
	}

	// Answer the keys a buffered change decides, and list the rest in index
	// order.
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	var order []int
	for i, key := range keys {
		if t.changes != nil {
			if c, ok := t.changes.Get(tentry{key: key}); ok {
				values[i], found[i] = bytes.Clone(c.data), !c.del
				continue
			}
		}
		order = append(order, i)
	}
	slices.SortFunc(order, func(i, j int) int { return bytes.Compare(keys[i], keys[j]) })
	sorted := make([][]byte, len(order))
	for j, i := range order {
		sorted[j] = keys[i]
		t.read(keys[i])
	}

	// Resolve them in the snapshot and read their values. An extent is read
	// into a new buffer, and an inline value is copied out of the shared
	// index page.
	vals, ok, err := t.db.lookupBatch(ctx, t.st, sorted)
	if err != nil {
		return nil, nil, err
	}
	for j, i := range order {
		if !ok[j] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		data, err := t.db.p.readValue(vals[j])
		if err != nil {
			return nil, nil, err
		}
		if !vals[j].isRef {
			data = bytes.Clone(data)
		}
		values[i], found[i] = data, true
	}
	return values, found, nil
}

// Prefetch loads the index pages of the snapshot on the path to each key,
// keeping the reads of each tree level in flight together.
func (t *Tx) Prefetch(ctx context.Context, keys [][]byte) error {
	// Check the transaction and keys.
	if t.done {
		return kvtx.ErrDiscarded
	}
	for _, key := range keys {
		if len(key) == 0 {
			return kvtx.ErrEmptyKey
		}
	}

	// Descend to the sorted keys together.
	sorted := slices.Clone(keys)
	slices.SortFunc(sorted, bytes.Compare)
	return t.db.prefetch(ctx, t.st, sorted)
}

// Exists reports whether key is present.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	// Require an active transaction and a nonempty key.
	switch {
	case t.done:
		return false, kvtx.ErrDiscarded
	case len(key) == 0:
		return false, kvtx.ErrEmptyKey
	}

	// Let a buffered change decide presence before reading the snapshot.
	if t.changes != nil {
		if c, ok := t.changes.Get(tentry{key: key}); ok {
			return !c.del, nil
		}
	}

	// Test presence in the pinned snapshot.
	t.read(key)
	_, found, err := t.db.lookup(t.st, key)
	return found, err
}

// Set buffers a write of key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	// Validate the transaction, key, and value before retaining the write.
	if err := t.writable(key); err != nil {
		return err
	}
	if int64(len(value)) > MaxValueSize {
		return ErrValueTooLarge
	}

	// Retain independent bytes until the transaction commits or discards.
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

// read records a snapshot read of key in an optimistic transaction.
func (t *Tx) read(key []byte) {
	if t.reads != nil {
		t.reads.addKey(key)
	}
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
		return ErrKeyTooLong
	}
	return nil
}

// ScanPrefix calls cb with each key and value under prefix in key order.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	// Hold an iterator over the snapshot and buffered changes.
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Deliver the prefix's key and value pairs in index order.
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
	// Hold an iterator over the snapshot and buffered changes.
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Deliver the prefix's keys in index order.
	for it.Next() {
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefixLens calls cb with each key under prefix and the length of its
// value in key order. It reads only the index, so it costs far less than
// ScanPrefix on large values.
func (t *Tx) ScanPrefixLens(ctx context.Context, prefix []byte, cb func(key []byte, n int) error) error {
	// Hold an iterator over the snapshot and buffered changes.
	it := t.iterate(prefix, false)
	defer it.Close()

	// Deliver the prefix's keys and value lengths in index order.
	for it.Next() {
		if err := cb(it.Key(), it.ValueLen()); err != nil {
			return err
		}
	}
	return it.Err()
}

// Iterate returns an iterator over keys under prefix, always in key order.
// When ctx is marked with block.WithBackground, the iterator leaves the
// index leaves it reads out of the page cache, so a background pass over many
// keys keeps the pages foreground lookups reuse.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	it := t.iterate(prefix, reverse)
	it.tree.scan = block.IsBackground(ctx)
	return it
}

// iterate returns an iterator over keys under prefix.
func (t *Tx) iterate(prefix []byte, reverse bool) *Iterator {
	if t.reads != nil {
		t.reads.addPrefix(prefix)
	}
	it := newIterator(t.db.p, t.st, t.changes, prefix, reverse)
	if t.done {
		it.err = kvtx.ErrDiscarded
	}
	return it
}

// Commit writes the changes and returns once they are durable, or commits
// them ordered when ctx carries kvtx.WithOrderedCommitContext.
func (t *Tx) Commit(ctx context.Context) error {
	return t.commit(ctx, kvtx.IsOrderedCommitContext(ctx))
}

// CommitOrdered writes the changes ordered after earlier commits without
// waiting for the drive to persist them.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	return t.commit(ctx, true)
}

// commit finishes the transaction, writing its changes.
func (t *Tx) commit(ctx context.Context, ordered bool) error {
	// A read-only transaction has nothing to write.
	if t.done {
		return kvtx.ErrDiscarded
	}
	if t.changes == nil {
		t.Discard()
		return nil
	}

	// Take the writer lock now for an optimistic transaction, moving it onto
	// the current state when its reads still hold there, then run the writes
	// it deferred to this point.
	db := t.db
	if t.reads != nil {
		// Reads alone serialize at the snapshot. A durable commit still
		// makes the commits its snapshot holds durable, as a locked one
		// does, so a caller that commits to flush what came before gets it.
		if t.changes.Len() == 0 && len(t.atCommit) == 0 {
			seq := t.st.seq
			t.Discard()
			if ordered {
				return nil
			}
			return db.flush.syncThrough(ctx, seq, 0)
		}
		if err := t.lockValidated(ctx); err != nil {
			t.done = true
			db.endWrite()
			return err
		}
		t.reads = nil
		for _, fn := range t.atCommit {
			if err := fn(ctx, t); err != nil {
				t.Discard()
				return err
			}
		}
		t.atCommit = nil
	}

	// Collect the changes in key order, counting the bytes they write.
	t.done = true
	defer db.endWrite()
	changes := make([]tentry, 0, t.changes.Len())
	t.changes.Scan(func(c tentry) bool {
		changes = append(changes, c)
		return true
	})

	// Write them, then run checkpoint and compaction work with the snapshot
	// released, release the writer lock, and wait for a checkpoint the
	// writes outpaced when they are background work.
	err := db.w.commit(ctx, t.st, changes, ordered, false)
	db.release(t.st, t.stripe)
	if err == nil {
		err = db.w.afterCommit(ctx)
	}
	seq := db.cur.Load().seq
	db.w.unlock(err)
	if err == nil && block.IsBackground(ctx) {
		err = db.w.waitBuild(ctx)
	}
	if err != nil || ordered {
		return err
	}

	// Flush outside the writer lock, sharing the flush with concurrent
	// commits.
	return db.flush.syncThrough(ctx, seq, 0)
}

// lockValidated takes the writer lock for an optimistic transaction and
// moves it onto the state the lock caught up to, when no commit since its
// snapshot changed a key it read. Otherwise it releases the lock and the
// snapshot and returns kvtx.ErrConflict. Before the lock it reads the index
// pages the commit resolves its changes in, so the commit finds them warm
// unless a checkpoint replaced the tree meanwhile.
func (t *Tx) lockValidated(ctx context.Context) error {
	// Warm the commit's pages; a failed read leaves them to the commit.
	db := t.db
	keys := make([][]byte, 0, t.changes.Len())
	t.changes.Scan(func(c tentry) bool {
		keys = append(keys, c.key)
		return true
	})
	_ = db.prefetch(ctx, t.st, keys)

	// Take the lock and the state it caught up to, in place of the
	// snapshot.
	if err := db.w.lock(ctx); err != nil {
		db.release(t.st, t.stripe)
		return err
	}
	snap := t.st.seq
	base, stripe := db.acquire()
	db.release(t.st, t.stripe)
	t.st, t.stripe = base, stripe

	// Refuse the commit when a later commit changed what it read.
	if db.recent.changed(snap, base.seq, t.reads) {
		db.release(base, stripe)
		db.w.unlock(nil)
		return kvtx.ErrConflict
	}
	return nil
}

// AtCommit defers fn to the commit of an optimistic transaction, after its
// reads validate, holding the writer lock, against the current state and this
// transaction's writes. A transaction holding the lock runs fn at once.
func (t *Tx) AtCommit(ctx context.Context, fn func(context.Context, kvtx.Tx) error) error {
	// Require a live write transaction.
	switch {
	case t.done:
		return kvtx.ErrDiscarded
	case t.changes == nil:
		return kvtx.ErrNotWrite
	}

	// Run fn now under the lock, or keep it for the commit.
	if t.reads == nil {
		return fn(ctx, t)
	}
	t.atCommit = append(t.atCommit, fn)
	return nil
}

// Discard ends the transaction without writing.
func (t *Tx) Discard() {
	// Leave an already finished transaction alone.
	if t.done {
		return
	}

	// Release the snapshot and any writer lock without publishing changes.
	t.done = true
	t.db.release(t.st, t.stripe)
	if t.changes != nil {
		if t.reads == nil {
			t.db.w.unlock(nil)
		}
		t.db.endWrite()
	}
}

// Stale reports whether another process has written a checkpoint newer than
// the one this snapshot reads. A handle with no pin cannot stop the writer
// reusing the pages of a snapshot two checkpoints old, so a read-only reader
// that finds its snapshot stale discards what it read from it, refreshes and
// reads again. A failed read of the superblock is reported as stale.
func (t *Tx) Stale() bool {
	sb, err := t.db.readSuper()
	return err != nil || sb.gen > t.st.gen
}

// _ is a type assertion
var (
	_ kvtx.OrderedCommitTx = (*Tx)(nil)
	_ kvtx.BatchTxOps      = (*Tx)(nil)
	_ kvtx.PrefetchTxOps   = (*Tx)(nil)
	_ kvtx.AtCommitTxOps   = (*Tx)(nil)
)
