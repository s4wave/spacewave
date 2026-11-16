//go:build evicthold

package block_store_writeback

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// holdStore times the SQLite writer holds of write transactions: from the
// transaction's open, after its writer turn and BEGIN IMMEDIATE, to its
// commit or discard. Background writes, the eviction pass's, and foreground
// writes are timed apart.
type holdStore struct {
	store_kvtx_sqlite.Store
	mtx      sync.Mutex
	bg, fg   holdTimes
	fgWait   holdTimes
	recordBg atomic.Bool
}

// holdTimes sums and bounds a set of durations.
type holdTimes struct {
	n          int
	total, max time.Duration
}

// add counts d.
func (h *holdTimes) add(d time.Duration) {
	h.n++
	h.total += d
	h.max = max(h.max, d)
}

// NewTransaction opens the real transaction and times a write's hold.
func (s *holdStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	start := time.Now()
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	bg := kvtx.IsBackgroundWriteContext(ctx)
	if !bg {
		s.mtx.Lock()
		s.fgWait.add(time.Since(start))
		s.mtx.Unlock()
	}
	return &holdTx{Tx: tx, store: s, bg: bg, start: time.Now()}, nil
}

// holdTx records its hold once it commits or discards.
type holdTx struct {
	kvtx.Tx
	store *holdStore
	bg    bool
	start time.Time
	done  bool
}

// end records the hold.
func (t *holdTx) end() {
	if t.done {
		return
	}
	t.done = true
	d := time.Since(t.start)
	t.store.mtx.Lock()
	defer t.store.mtx.Unlock()
	switch {
	case !t.bg:
		t.store.fg.add(d)
	case t.store.recordBg.Load():
		t.store.bg.add(d)
	}
}

// Commit commits and records the hold.
func (t *holdTx) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	t.end()
	return err
}

// Discard discards and records the hold.
func (t *holdTx) Discard() {
	t.Tx.Discard()
	t.end()
}

// TestEvictRefusalWriterHold measures, on a SQLite-backed store with a
// foreground writer running, the writer holds of one pass over an
// all-refusal batch of 1,024 and of 8,192 entries, refusal retry on against
// off. It reports numbers and asserts nothing about them. Run with
// -tags evicthold.
func TestEvictRefusalWriterHold(t *testing.T) {
	for _, n := range []int{1024, 8192} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("n=%d/retry=%t", n, retry), func(t *testing.T) {
				measureRefusalHold(t, n, retry)
			})
		}
	}
}

// measureRefusalHold runs one measurement of TestEvictRefusalWriterHold.
func measureRefusalHold(t *testing.T, n int, retry bool) {
	// Open a Volume on a SQLite file and a writeback store on it.
	ctx := t.Context()
	SetEvictRefusedRetry(retry)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	SetEvictBatch(EvictBatch{MinBlocks: n, MaxBlocks: n})
	t.Cleanup(func() { SetEvictBatch(EvictBatch{}) })
	sq, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "hold.db"), "kv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	hs := &holdStore{Store: sq}
	vol, err := volume_kvtx.NewVolume(ctx, "test/hold", store_kvkey.NewDefaultKVKey(), hs, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	s, err := NewStore(ctx, ownedStore{vol}, vol, markers, testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}

	// Queue n unmarked fills the remote lacks, so every entry is refused.
	for i := range n {
		data := fmt.Appendf(nil, "hold-refusal-%06d-%0100d", i, 0)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		s.Refill(ref, &block.StoredBlock{Data: data, RefsKnown: true})
	}
	waitStatus(t, s, func(st Status) bool { return st.Refills == int64(n) })

	// Run a foreground writer of one key per transaction during the pass.
	wctx, stop := context.WithCancel(ctx)
	var writes atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; wctx.Err() == nil; i++ {
			_ = kvtx.RunTransaction(wctx, true, func(ctx context.Context) (kvtx.Tx, error) {
				return markers.NewTransaction(ctx, true)
			}, func(ctx context.Context, tx kvtx.Tx) error {
				return tx.Set(ctx, fmt.Appendf(nil, "fg/%d", i%64), []byte("x"))
			})
			writes.Add(1)
		}
	}()
	time.Sleep(50 * time.Millisecond)
	hs.mtx.Lock()
	hs.fg, hs.fgWait = holdTimes{}, holdTimes{}
	hs.mtx.Unlock()
	writes.Store(0)

	// Run the pass, timing its writer holds.
	hs.recordBg.Store(true)
	start := time.Now()
	result, err := s.Evict(ctx, newInmemBlockStore(), 1<<40, nil, nil)
	took := time.Since(start)
	hs.recordBg.Store(false)
	stop()
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if result.Refused != n {
		t.Fatalf("result = %+v, want %d refusals", result, n)
	}

	// Report the holds.
	hs.mtx.Lock()
	defer hs.mtx.Unlock()
	t.Logf("n=%d retry=%t pass=%v: eviction writes %d, hold max %v total %v; foreground writes %d during the pass, hold max %v, wait max %v total %v",
		n, retry, took.Round(time.Microsecond), hs.bg.n, hs.bg.max.Round(time.Microsecond), hs.bg.total.Round(time.Microsecond),
		writes.Load(), hs.fg.max.Round(time.Microsecond), hs.fgWait.max.Round(time.Microsecond), hs.fgWait.total.Round(time.Microsecond))
}
