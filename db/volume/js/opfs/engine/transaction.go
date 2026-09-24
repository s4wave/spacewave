package engine

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// transaction buffers bounded mutations against one protected committed snapshot.
// Commit or Discard releases the immutable files retained by the first read.
type transaction struct {
	// engine owns durable data and publication.
	engine *Engine
	// id identifies the transaction in workload records.
	id uint64
	// write permits pending mutations.
	write bool
	// metadata confines conflict validation to the public metadata namespace.
	metadata bool
	// discarded closes the transaction and all derived iterators.
	discarded bool
	// snapshot retains the first committed view until the transaction ends.
	snapshot *snapshot
	// pending provides read-your-writes within explicit memory bounds.
	pending map[string]*Record
	// pendingBytes charges retained mutation keys, values, and record overhead.
	pendingBytes int
}

// revision selects the logical state visible through this transaction's store.
func (t *transaction) revision(root *Root) uint64 {
	if t.metadata {
		return root.MetadataRevision
	}
	return root.Revision
}

// readSnapshot acquires the transaction's committed view on its first read.
func (t *transaction) readSnapshot(ctx context.Context) (*snapshot, error) {
	if t.snapshot == nil {
		snapshot, err := t.engine.snapshot(ctx)
		if err != nil {
			return nil, err
		}
		t.snapshot = snapshot
	}
	return t.snapshot, nil
}

// check validates the transaction lifetime and caller cancellation.
func (t *transaction) check(ctx context.Context) error {
	if t.discarded {
		return kvtx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	t.engine.mtx.Lock()
	closed := t.engine.closed
	t.engine.mtx.Unlock()
	if closed {
		return ErrClosed
	}
	return nil
}

// Get returns a copied committed value or the transaction's pending value.
func (t *transaction) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	value, found, err := t.get(ctx, key)
	workload.Record{Op: workload.OpGet, ID: t.id, Size: foundSize(found, len(value)), Key: key}.Log(ctx)
	return value, found, err
}

// Exists checks the same generation-consistent record view as Get.
func (t *transaction) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.get(ctx, key)
	workload.Record{Op: workload.OpExists, ID: t.id, Size: presence(found), Key: key}.Log(ctx)
	return found, err
}

// get reads the pending value or the protected committed snapshot.
func (t *transaction) get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if err := t.check(ctx); err != nil {
		return nil, false, err
	}
	snapshot, err := t.readSnapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	if pending := t.pending[string(key)]; pending != nil {
		return bytes.Clone(pending.Value), !pending.Deleted, nil
	}
	value, found, err := snapshot.get(ctx, key)
	return bytes.Clone(value), found, err
}

// Set records a bounded caller-owned mutation for commit.
func (t *transaction) Set(ctx context.Context, key, value []byte) error {
	workload.Record{Op: workload.OpSet, ID: t.id, Size: int64(len(value)), Key: key}.Log(ctx)
	return t.mutate(ctx, key, value, false)
}

// Delete hides a key and remains a no-op when that key is absent.
func (t *transaction) Delete(ctx context.Context, key []byte) error {
	workload.Record{Op: workload.OpDelete, ID: t.id, Key: key}.Log(ctx)
	return t.mutate(ctx, key, nil, true)
}

// mutate retains bounded copied input; blind writes observe no committed state.
func (t *transaction) mutate(ctx context.Context, key, value []byte, deleted bool) error {
	if err := t.check(ctx); err != nil {
		return err
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if len(key) > maxKeyBytes || len(value) > MaxValueBytes {
		return ErrLimit
	}
	previous := t.pending[string(key)]
	count, size := len(t.pending)+1, t.pendingBytes+len(key)+len(value)+32
	if previous != nil {
		count--
		size -= len(previous.Key) + len(previous.Value) + 32
	}
	if count > maxBatchRecords || size > maxBatchBytes {
		return ErrLimit
	}
	t.pending[string(key)] = &Record{Key: bytes.Clone(key), Value: bytes.Clone(value), Deleted: deleted}
	t.pendingBytes = size
	return nil
}

// Size counts visible keys with a bounded cursor.
func (t *transaction) Size(ctx context.Context) (uint64, error) {
	var count uint64
	err := t.ScanPrefixKeys(ctx, nil, func([]byte) error {
		count++
		return nil
	})
	return count, err
}

// ScanPrefix visits ordered visible records without retaining the whole answer.
func (t *transaction) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		value, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(it.Key(), value); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefixKeys visits keys through the same bounded transaction cursor.
func (t *transaction) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, _ []byte) error { return cb(key) })
}

// Iterate merges bounded pending changes with a lazy committed partition cursor.
func (t *transaction) Iterate(ctx context.Context, prefix []byte, _ bool, reverse bool) kvtx.Iterator {
	pending := make([]*Record, 0, len(t.pending))
	for _, record := range t.pending {
		if bytes.HasPrefix(record.Key, prefix) {
			pending = append(pending, record)
		}
	}
	slices.SortFunc(pending, func(a, b *Record) int { return bytes.Compare(a.Key, b.Key) })
	it := &iterator{ctx: ctx, tx: t, id: t.engine.workloadIDs.Add(1), prefix: bytes.Clone(prefix), reverse: reverse, pending: pending}
	var order int64
	if reverse {
		order = 1
	}
	workload.Record{Op: workload.OpIterate, ID: it.id, Parent: t.id, Size: order, Key: prefix}.Log(ctx)
	return it
}

// Commit validates the complete observed generation and durably publishes writes.
func (t *transaction) Commit(ctx context.Context) error {
	workload.Record{Op: workload.OpCommit, ID: t.id, Size: int64(len(t.pending))}.Log(ctx)
	if err := t.check(ctx); err != nil {
		return err
	}
	if !t.write || len(t.pending) == 0 {
		t.release()
		return nil
	}
	records := make([]*Record, 0, len(t.pending))
	for _, record := range t.pending {
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b *Record) int { return bytes.Compare(a.Key, b.Key) })
	// Blind writes serialize against the current root without a stale read dependency.
	var base *uint64
	if t.snapshot != nil {
		revision := t.revision(t.snapshot.root)
		base = &revision
	}
	err := t.engine.apply(ctx, base, records, t.metadata)
	t.release()
	return err
}

// Discard releases the snapshot and mutations and invalidates derived iterators.
func (t *transaction) Discard() {
	workload.Record{Op: workload.OpDiscard, ID: t.id}.Log(context.Background())
	t.release()
}

// release ends the transaction, dropping its snapshot and mutations.
func (t *transaction) release() {
	if t.snapshot != nil {
		t.snapshot.release()
		t.snapshot = nil
	}
	t.discarded = true
	t.pending = nil
	t.pendingBytes = 0
}

// _ verifies the transaction interface.
var _ kvtx.Tx = (*transaction)(nil)
