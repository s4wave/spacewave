package kvtx

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/tidwall/btree"
)

// maxWriteTombstones allows one deleted singleton in a bounded probe. A
// cursor-led log walk needs no committed tombstones. Deletes in the current
// slice have not committed, so they do not count toward this limit, and
// neither do the crossings of a scan writerScans names.
const maxWriteTombstones = 1

// seekGuard reports a writer that iterates over too many deleted liveness keys.
// The deletion history follows the backing store across Volume opens.
type seekGuard struct {
	// Store opens the backing transactions.
	kvtx.Store
	// t receives failures even when the caller handles the transaction error.
	t *testing.T
	// mtx orders committed deletion history across writer handoffs.
	mtx sync.Mutex
	// deleted tracks committed deletes until a committed write sets the key.
	deleted *btree.BTreeG[string]
	// trace holds every committed write in commit order, when
	// writeTraceDir names a directory, as writeTrace describes.
	trace []string
}

// writeTraceEnv names the directory where each test writes the trace of its
// stores' committed writes. Two builds whose runs write equal traces made the
// same writes, in the same order, with the same values.
const writeTraceEnv = "SPACEWAVE_WRITE_TRACE_DIR"

// writeTraceDir is the directory writeTraceEnv names, or empty when unset.
var writeTraceDir = os.Getenv(writeTraceEnv)

// writeTraceSeqs numbers the guarded stores of each test in the order they
// open, so each store of a test writes its own trace file.
var writeTraceSeqs sync.Map // test name -> *atomic.Int64

// newSeekGuard guards the write transactions of store.
func newSeekGuard(t *testing.T, store kvtx.Store) *seekGuard {
	g := &seekGuard{Store: store, t: t, deleted: btree.NewBTreeG(cmp.Less[string])}
	if writeTraceDir != "" {
		seqs, _ := writeTraceSeqs.LoadOrStore(t.Name(), new(atomic.Int64))
		seq := seqs.(*atomic.Int64).Add(1)
		t.Cleanup(func() { g.writeTrace(seq) })
	}
	return g
}

// writeTrace writes the store's committed writes to a file named by the test
// and the store's number in the test.
func (s *seekGuard) writeTrace(seq int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(s.t.Name())
	path := filepath.Join(writeTraceDir, fmt.Sprintf("%s.%d.trace", name, seq))
	if err := os.WriteFile(path, []byte(strings.Join(s.trace, "")), 0o644); err != nil {
		s.t.Error(err)
	}
}

// newGuardedStore builds the store shared by in-memory Volume test opens.
func newGuardedStore(t *testing.T) *seekGuard {
	return newSeekGuard(t, store_kvtx_inmem.NewStore())
}

// NewTransaction guards iteration only in a write transaction.
func (s *seekGuard) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Open the backing store's snapshot or acquire its writer turn.
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}

	// Keep this write's mutations separate until it commits. An optimistic
	// native store takes the writer turn only in its commit callback.
	_, canDefer := tx.(kvtx.AtCommitTxOps)
	return &seekGuardTx{
		Tx:         tx,
		store:      s,
		changed:    make(map[string]bool),
		reported:   make(map[string]bool),
		optimistic: canDefer && kvtx.IsOptimisticContext(ctx),
	}, nil
}

// SupportsAtomicCommit forwards the store's atomicity contract.
func (s *seekGuard) SupportsAtomicCommit() bool {
	store, ok := s.Store.(kvtx.AtomicCommitStore)
	return ok && store.SupportsAtomicCommit()
}

// WritersWaiting forwards the backing store's writer queue.
func (s *seekGuard) WritersWaiting() bool {
	store, ok := s.Store.(kvtx.WriterQueueStore)
	return ok && store.WritersWaiting()
}

// orderedSeekGuard preserves ordered commits on a durable test store.
type orderedSeekGuard struct {
	*seekGuard
	// ordered is the backing store's durability fence.
	ordered kvtx.OrderedCommitStore
}

// newOrderedSeekGuard guards a store without changing its durability contract.
func newOrderedSeekGuard(t *testing.T, store kvtx.OrderedCommitStore) *orderedSeekGuard {
	return &orderedSeekGuard{seekGuard: newSeekGuard(t, store), ordered: store}
}

// Sync makes the backing store's commits durable.
func (s *orderedSeekGuard) Sync(ctx context.Context) error {
	return s.ordered.Sync(ctx)
}

// WaitDurable waits for the backing store's durability fence.
func (s *orderedSeekGuard) WaitDurable(ctx context.Context) error {
	return s.ordered.WaitDurable(ctx)
}

// seekGuardTx tracks mutations and iteration in one write transaction.
type seekGuardTx struct {
	// Tx owns the actual writes.
	kvtx.Tx
	// store holds the committed deletion history.
	store *seekGuard
	// changed maps liveness keys to whether this transaction deletes them.
	changed map[string]bool
	// crossed counts recorded deletions stepped over by this transaction,
	// outside the scans writerScans names.
	crossed int
	// reported holds each named scan this transaction already logged.
	reported map[string]bool
	// optimistic reports that the write body runs against a read snapshot.
	optimistic bool
	// ops are the transaction's writes in order, when writeTraceDir is set.
	ops []string
}

// Set records a key to forget from the deletion history at commit.
func (t *seekGuardTx) Set(ctx context.Context, key, value []byte) error {
	// Stage the value in the backing transaction.
	if err := t.Tx.Set(ctx, key, value); err != nil {
		return err
	}
	if writeTraceDir != "" {
		t.ops = append(t.ops, fmt.Sprintf("S %x %x\n", key, value))
	}

	// A committed set supersedes a previous deletion.
	if bytes.HasPrefix(key, livenessPrefix) {
		t.changed[string(key)] = false
	}
	return nil
}

// Delete records a key to add to the deletion history at commit.
func (t *seekGuardTx) Delete(ctx context.Context, key []byte) error {
	// Stage the deletion in the backing transaction.
	if err := t.Tx.Delete(ctx, key); err != nil {
		return err
	}
	if writeTraceDir != "" {
		t.ops = append(t.ops, fmt.Sprintf("D %x\n", key))
	}

	// Preserve the transaction's final operation for this key.
	if bytes.HasPrefix(key, livenessPrefix) {
		t.changed[string(key)] = true
	}
	return nil
}

// GetBatch preserves the backing store's batch read.
func (t *seekGuardTx) GetBatch(ctx context.Context, keys [][]byte) ([][]byte, []bool, error) {
	return kvtx.GetBatch(ctx, t.Tx, keys)
}

// ExistsBatch preserves the backing store's batch presence probe.
func (t *seekGuardTx) ExistsBatch(ctx context.Context, keys [][]byte) ([]bool, error) {
	return kvtx.ExistsBatch(ctx, t.Tx, keys)
}

// Commit publishes the mutations with a full commit.
func (t *seekGuardTx) Commit(ctx context.Context) error {
	return t.commit(ctx, func() error { return t.Tx.Commit(ctx) })
}

// CommitOrdered publishes the mutations with an ordered commit.
func (t *seekGuardTx) CommitOrdered(ctx context.Context) error {
	return t.commit(ctx, func() error { return kvtx.CommitOrdered(ctx, t.Tx) })
}

// commit publishes successful mutations before the next writer can count them.
func (t *seekGuardTx) commit(ctx context.Context, commit func() error) error {
	// Only take the history lock once this transaction holds the writer.
	locked := false
	lock := func() {
		t.store.mtx.Lock()
		locked = true
	}
	defer func() {
		if locked {
			t.store.mtx.Unlock()
		}
	}()
	switch {
	case t.optimistic:
		if len(t.changed) == 0 && len(t.ops) == 0 {
			return commit()
		}
		if err := kvtx.AtCommit(ctx, t.Tx, func(context.Context, kvtx.Tx) error {
			lock()
			return nil
		}); err != nil {
			return err
		}
	default:
		lock()
	}

	// Publish the final operation for each liveness key on success.
	if err := commit(); err != nil {
		return err
	}
	if len(t.ops) != 0 {
		t.store.trace = append(append(t.store.trace, "C\n"), t.ops...)
	}
	for key, deleted := range t.changed {
		if deleted {
			t.store.deleted.Set(key)
			continue
		}
		t.store.deleted.Delete(key)
	}
	return nil
}

// check counts committed deletions crossed in the iterator's prefix on one
// move. from is included on a seek and excluded when advancing past it.
// A nil endpoint denotes the corresponding end of the prefix.
func (t *seekGuardTx) check(prefix, from, to []byte, reverse, advance bool) error {
	// Clip the gap to the prefix and exclude any key the iterator found.
	end, _ := kvtx.PrefixSuccessor(prefix)
	low, high := max(string(prefix), string(from)), string(to)
	skipLow, skipHigh := advance && len(from) != 0, true
	if to == nil {
		high = string(end)
	}
	if reverse {
		low, high = max(string(prefix), string(to)), string(from)
		skipLow, skipHigh = to != nil, advance && len(from) != 0
		if from == nil || end != nil && high >= string(end) {
			high, skipHigh = string(end), true
		}
	}

	// Count only the deleted keys inside this move's range.
	var n int
	t.store.mtx.Lock()
	t.store.deleted.Ascend(low, func(key string) bool {
		if high != "" && (key > high || skipHigh && key == high) {
			return false
		}
		if key != low || !skipLow {
			n++
		}
		return true
	})
	t.store.mtx.Unlock()
	if n == 0 {
		return nil
	}

	// Leave a named scan's crossings off the transaction's count, so a
	// later iteration of the same transaction is judged on its own, and
	// log the scan once so a run shows which of them it met.
	if scan := writerScan(); scan != "" {
		if !t.reported[scan] {
			t.store.t.Logf("%s crossed %d deleted liveness keys at prefix %x", scan, n, prefix)
			t.reported[scan] = true
		}
		return nil
	}
	t.crossed += n
	if t.crossed <= maxWriteTombstones {
		return nil
	}

	// Fail the test even if the caller treats the scan error as recoverable.
	err := errors.Errorf("write transaction crossed %d deleted liveness keys (limit %d) at prefix %x from %x to %x",
		t.crossed, maxWriteTombstones, prefix, from, to)
	t.store.t.Errorf("%v\n%s", err, debug.Stack())
	return err
}

// writerScans names the scans that still iterate deleted keys inside a write
// transaction: the function that runs each, and the Records calls through
// which it scans. The guard passes these and no others, so deleting an entry
// is the check that its scan has moved out of the writer.
//
// The adoption's count passes read a slice of a liveness kind and count it
// in one transaction, so that each record is counted once against the Lands
// between slices. The lease rotation lists every root of the lapsed lease in
// the commit that moves them to the new one, which keeps the roots held
// across the rotation. The stage release lists the stage's roots in the
// commit that releases them.
var writerScans = map[string][]string{
	gcPackage + ".(*adoption).settleLists": {gcPackage + ".(*Records).scanAfter", gcPackage + ".(*Records).scanThrough"},
	gcPackage + ".scanLists":               {gcPackage + ".(*Records).scanAfter", gcPackage + ".(*Records).scanThrough"},
	gcPackage + ".(*adoption).countRoots":  {gcPackage + ".(*Records).scanNamesAfter"},
	kvtxPackage + ".rehold":                {gcPackage + ".(*Records).ListLeaseRoots"},
	kvtxPackage + ".listStageRoots":        {gcPackage + ".(*Records).ListRoots"},
}

// gcPackage and kvtxPackage are the import paths of the functions writerScans
// names.
const (
	gcPackage   = "github.com/s4wave/spacewave/db/block/gc"
	kvtxPackage = "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// writerScan returns the short name of the writerScans entry that makes this
// iteration, or the empty string for an iteration none of them makes.
func writerScan() string {
	// Read the iterator's call stack, innermost call first.
	var callers [32]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	var stack []string
	for {
		frame, more := frames.Next()
		stack = append(stack, frame.Function)
		if !more {
			return namedScan(stack)
		}
	}
}

// namedScan returns the short name of the writerScans entry that makes the
// iteration below stack, whose innermost call comes first, or the empty
// string. An entry makes the iteration only when it called one of its Records
// calls directly and none of its own closures runs beneath that call: a scan
// by another callee, or by a callback the Records call invokes, is charged
// like any other.
func namedScan(stack []string) string {
	for i, fn := range stack {
		// Walk outward to the innermost named function.
		via, ok := writerScans[fn]
		if !ok {
			continue
		}

		// Charge a scan it reaches through any other call.
		if i == 0 || !slices.Contains(via, stack[i-1]) {
			return ""
		}

		// Charge a scan one of its callbacks makes.
		if slices.ContainsFunc(stack[:i], func(inner string) bool { return strings.HasPrefix(inner, fn+".") }) {
			return ""
		}
		return fn[strings.LastIndexByte(fn, '.')+1:]
	}
	return ""
}

// TestNamedScan checks that the guard passes a named scan only for the
// iteration that scan makes itself.
func TestNamedScan(t *testing.T) {
	iter := kvtxPackage + ".(*seekGuardIterator).Next"
	scan := gcPackage + ".(*Records).scan"
	for _, c := range []struct {
		// name describes the call stack.
		name string
		// stack is the call stack, innermost call first.
		stack []string
		// want is the named scan the guard passes, or empty to charge.
		want string
	}{
		{
			name:  "the scan a count pass makes",
			stack: []string{iter, scan, gcPackage + ".(*Records).scanAfter", gcPackage + ".(*adoption).settleLists", gcPackage + ".(*adoption).pass"},
			want:  "settleLists",
		},
		{
			name:  "a scan in the body a list slice runs",
			stack: []string{iter, scan, gcPackage + ".(*Records).addFront", gcPackage + ".(*adoption).pass.func1", gcPackage + ".scanLists"},
		},
		{
			name:  "a scan in another callee of the rotation",
			stack: []string{iter, scan, gcPackage + ".(*Records).Held", kvtxPackage + ".rehold"},
		},
		{
			name:  "a scan in the callback of the stage release's list",
			stack: []string{iter, scan, kvtxPackage + ".listStageRoots.func1", gcPackage + ".(*Records).ListRoots", kvtxPackage + ".listStageRoots"},
		},
		{
			name:  "a scan no entry names",
			stack: []string{iter, scan, gcPackage + ".(*Collector).netLogs"},
		},
	} {
		if got := namedScan(c.stack); got != c.want {
			t.Errorf("%s: namedScan = %q, want %q", c.name, got, c.want)
		}
	}
}

// Iterate returns an iterator that charges gaps between live keys to the writer.
func (t *seekGuardTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	// A read snapshot consumes no writer time.
	it := t.Tx.Iterate(ctx, prefix, sort, reverse)
	if t.optimistic {
		return it
	}

	// Attribute each movement of a writer's cursor to that transaction.
	return &seekGuardIterator{Iterator: it, tx: t, prefix: prefix, reverse: reverse}
}

// ScanPrefixKeys charges each visited part of the prefix to the writer.
func (t *seekGuardTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	// Use the same movement checks as a caller-owned iterator.
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Stop at the callback's error without charging the unvisited suffix.
	for it.Next() {
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefix checks iteration and reads each key's current value for the callback.
func (t *seekGuardTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		value, found, err := t.Get(ctx, key)
		if err != nil || !found {
			return err
		}
		return cb(key, value)
	})
}

// seekGuardIterator checks movement within a writer's prefix.
type seekGuardIterator struct {
	// Iterator owns the underlying cursor.
	kvtx.Iterator
	// tx accumulates the cost of every iterator in the transaction.
	tx *seekGuardTx
	// prefix bounds this iteration.
	prefix []byte
	// reverse reports whether the cursor moves toward smaller keys.
	reverse bool
	// started is set after the first successful move attempt.
	started bool
	// err records a guard failure after the underlying cursor closes.
	err error
}

// Err returns the first error that ended iteration.
func (i *seekGuardIterator) Err() error {
	return cmp.Or(i.err, i.Iterator.Err())
}

// Seek checks the gap from the requested key to the cursor's destination.
func (i *seekGuardIterator) Seek(key []byte) error {
	// Shift the underlying cursor before counting skipped records.
	if err := i.Iterator.Seek(key); err != nil {
		return cmp.Or(i.err, err)
	}

	// Count from the requested position even when the prefix is exhausted.
	i.started = true
	return i.moved(key, false)
}

// Next checks the gap from the previous position to the next live key.
func (i *seekGuardIterator) Next() bool {
	// Take the first move or advance from a live key only once.
	if i.started && !i.Valid() {
		return false
	}
	from := bytes.Clone(i.Key())
	found := i.Iterator.Next()
	i.started = true

	// Charge the gap, including an exhausted prefix's last interval.
	err := i.moved(from, true)
	return found && err == nil
}

// moved charges the skipped range after a successful cursor operation.
func (i *seekGuardIterator) moved(from []byte, advance bool) error {
	// Propagate errors from the backing iterator as its caller expects.
	if i.Iterator.Err() != nil {
		return nil
	}

	// Close the backing cursor on the first guard failure.
	i.err = i.tx.check(i.prefix, from, i.Key(), i.reverse, advance)
	if i.err != nil {
		i.Close()
	}
	return i.err
}

// _ checks the store, transaction and iterator contracts.
var (
	_ kvtx.AtomicCommitStore  = (*seekGuard)(nil)
	_ kvtx.WriterQueueStore   = (*seekGuard)(nil)
	_ kvtx.OrderedCommitStore = (*orderedSeekGuard)(nil)
	_ kvtx.OrderedCommitTx    = (*seekGuardTx)(nil)
	_ kvtx.Iterator           = (*seekGuardIterator)(nil)
)
