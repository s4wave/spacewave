package kvtx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type testGate struct {
	started, release chan struct{}
	once             sync.Once
}

func (g *testGate) unblock() { g.once.Do(func() { close(g.release) }) }

type testStore struct {
	db_kvtx.Store
	mu                   sync.Mutex
	gate                 *testGate
	failSet, errorCommit error
	afterCommit          bool
	writersWaiting       bool
	commits              int
	// readDelay slows each read of a write transaction, as a cold page does.
	readDelay time.Duration
	// longestWrite is the longest a write transaction stayed open.
	longestWrite time.Duration
}

// holdWrite records the time a write transaction stayed open.
func (s *testStore) holdWrite(opened time.Time) {
	s.mu.Lock()
	s.longestWrite = max(s.longestWrite, time.Since(opened))
	s.mu.Unlock()
}

func (s *testStore) SupportsAtomicCommit() bool { return true }

// WritersWaiting reports the injected writer queue, so a sweep transaction
// yields after one block.
func (s *testStore) WritersWaiting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writersWaiting
}
func (s *testStore) arm(t *testing.T) *testGate {
	// Arm the publication store gate and register its release.
	g := &testGate{started: make(chan struct{}), release: make(chan struct{})}
	s.mu.Lock()
	s.gate = g
	s.mu.Unlock()
	t.Cleanup(g.unblock)
	return g
}

func (s *testStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	// Capture the publication gate and injected write faults.
	var g *testGate
	var setErr, commitErr error
	var after bool
	var delay time.Duration
	if write {
		s.mu.Lock()
		g = s.gate
		s.gate = nil
		setErr = s.failSet
		commitErr = s.errorCommit
		after = s.afterCommit
		delay = s.readDelay
		s.mu.Unlock()
	}

	// Hold the physical transaction until the test releases its gate.
	if g != nil {
		close(g.started)
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Open the underlying transaction and attach write fault injection.
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write {
		return tx, nil
	}
	return &testStoreTx{Tx: tx, store: s, failSet: setErr, errorCommit: commitErr, afterCommit: after, delay: delay, opened: time.Now()}, nil
}

type testStoreTx struct {
	db_kvtx.Tx
	store                *testStore
	failSet, errorCommit error
	afterCommit          bool
	// delay slows each read, and opened times the transaction's hold.
	delay  time.Duration
	opened time.Time
	ended  sync.Once
}

// end records the transaction's hold once, on commit or discard.
func (t *testStoreTx) end() {
	t.ended.Do(func() { t.store.holdWrite(t.opened) })
}

func (t *testStoreTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	time.Sleep(t.delay)
	return t.Tx.Get(ctx, key)
}

func (t *testStoreTx) Exists(ctx context.Context, key []byte) (bool, error) {
	time.Sleep(t.delay)
	return t.Tx.Exists(ctx, key)
}

func (t *testStoreTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	time.Sleep(t.delay)
	return t.Tx.ScanPrefixKeys(ctx, prefix, cb)
}

func (t *testStoreTx) Discard() {
	t.end()
	t.Tx.Discard()
}

func (t *testStoreTx) Set(ctx context.Context, key, value []byte) error {
	if err := t.Tx.Set(ctx, key, value); err != nil {
		return err
	}
	if t.failSet != nil && strings.HasSuffix(string(key), "/head") {
		return t.failSet
	}
	return nil
}

func (t *testStoreTx) Commit(ctx context.Context) error {
	// Inject failures before committing the publication transaction.
	if t.errorCommit != nil && !t.afterCommit {
		return t.errorCommit
	}
	err := t.Tx.Commit(ctx)
	t.end()
	if err != nil {
		return err
	}

	// Count committed physical transactions before returning the injected result.
	t.store.mu.Lock()
	t.store.commits++
	t.store.mu.Unlock()
	return t.errorCommit
}

func newTestVolume(t *testing.T) (*Volume, *testStore) {
	// Create an atomic publication Volume with transaction fault injection.
	t.Helper()
	s := &testStore{Store: store_kvtx_inmem.NewStore()}
	v, err := NewVolume(t.Context(), "test/volume", store_kvkey.NewDefaultKVKey(), s, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	return v, s
}

// leafOpts returns put options declaring a block with no children.
func leafOpts() *block.PutOpts {
	return &block.PutOpts{RefsKnown: true}
}

// batchFor returns a batch of one leaf block named for id and next.
func batchFor(t *testing.T, id, next string) []*block.PutBatchEntry {
	t.Helper()
	data := []byte("block for " + id + "/" + next)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}
}

// putBatch stores entries through v and returns only its error.
func putBatch(ctx context.Context, v *Volume, entries []*block.PutBatchEntry) error {
	_, err := v.PutBlockBatch(ctx, entries)
	return err
}
