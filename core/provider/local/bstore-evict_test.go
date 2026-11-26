package provider_local

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
)

// evictTestBackend fails the first remote confirmation, then delegates.
type evictTestBackend struct {
	backendStore
	// failed records whether the one transient failure has been returned.
	failed bool
}

// GetBlockExistsBatch returns a sentinel failure before confirming any block.
func (b *evictTestBackend) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Fail before any local value can be dropped.
	if !b.failed {
		b.failed = true
		return nil, block.ErrUnavailable
	}

	// Confirm only the actual backend's blocks on subsequent attempts.
	return b.backendStore.GetBlockExistsBatch(ctx, refs)
}

// TestEvictionRetriesTransientBackend uses the production routine backoff to
// retry a backend error without a restart and logs only the successful work.
func TestEvictionRetriesTransientBackend(t *testing.T) {
	// Upload through the real test backend and fail its first eviction probe.
	f := newFallbackTest(t)
	if _, _, err := f.wb.PutBlock(f.ctx, []byte("retry the backend probe"), &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	f.upload()
	f.p.remote = new(atomic.Pointer[openBackend])
	f.p.remote.Store(&openBackend{store: &evictTestBackend{backendStore: f.packs}})
	f.p.evictGate = new(csync.Mutex)
	f.wb.SetPolicy(block_store_writeback.Policy{ResidentHigh: 1})
	logger, hook := logrustest.NewNullLogger()
	le := logrus.NewEntry(logger).WithFields(logrus.Fields{
		"bstore-id": "test-store", "account-id": "test-account", "routine": "storage-evict",
	})
	exits := make(chan error, 2)
	rc := routine.NewRoutineContainer(routine.WithRetry(providerBackoff), routine.WithExitCb(func(err error) { exits <- err }))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	defer rc.ClearContext()
	rc.SetRoutine(func(ctx context.Context) error { return f.p.runEviction(ctx, le) })
	rc.SetContext(ctx, true)

	// The first pass returns the exact backend error, and its successor
	// drains the preserved queue through the routine's existing retry path.
	select {
	case err := <-exits:
		if !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("first pass error = %v, want unavailable", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("first pass never returned")
	}
	deadline := time.After(10 * time.Second)
	for {
		status, changed := f.wb.GetStatus()
		if status.EvictableBytes == 0 {
			break
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatal("transient backend error was not retried")
		}
	}

	// Join the routine to flush the successful pass before its minute ends.
	cancel()
	select {
	case err := <-exits:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("routine exit = %v, want canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("eviction routine never stopped")
	}
	// Each run of the routine says its first pass starts, and only the pass
	// that evicted logs counts.
	var entries []*logrus.Entry
	starts := 0
	for _, entry := range hook.AllEntries() {
		if entry.Message == "eviction pass starting" {
			if entry.Data["cause"] != "start" {
				t.Fatalf("pass start = %+v, want cause start", entry.Data)
			}
			starts++
			continue
		}
		entries = append(entries, entry)
	}
	if starts != 2 {
		t.Fatalf("logged %d pass starts, want 2", starts)
	}
	if len(entries) != 1 {
		t.Fatalf("logged %d summaries, want 1", len(entries))
	}
	for _, entry := range entries {
		if entry.Data["refused"] != 0 || entry.Data["requeued"] != 0 {
			t.Fatalf("pass counters = %v", entry.Data)
		}
		if entry.Message != "eviction pass" || len(entry.Data) != len(le.Data)+4 {
			t.Fatalf("pass log adds fields or text beyond counters: %+v", entry)
		}
		for key, value := range le.Data {
			if entry.Data[key] != value {
				t.Fatalf("pass log changed inherited field %s: %v", key, entry.Data)
			}
		}
		if _, ok := entry.Data["evicted"].(int); !ok {
			t.Fatalf("pass eviction count = %v", entry.Data)
		}
		if _, ok := entry.Data["evicted-bytes"].(int64); !ok {
			t.Fatalf("pass byte count = %v", entry.Data)
		}
	}
	if entries[0].Data["evicted"] != 1 {
		t.Fatalf("successful pass counters = %v", entries[0].Data)
	}
}

// TestEvictionPassLogIntervals checks silence for zero passes, minute totals,
// and the final partial interval through the real provider routine.
func TestEvictionPassLogIntervals(t *testing.T) {
	for _, name := range []string{"zero", "accumulate"} {
		t.Run(name, func(t *testing.T) {
			// The Store watches process-wide policy outside synthetic time. Its
			// worker must not become an external-channel waiter inside the bubble.
			ownerCtx, stopOwner := context.WithCancel(t.Context())
			defer stopOwner()
			local := newLeafStore()
			wb, err := block_store_writeback.NewStore(ownerCtx, local, local, store_kvtx_inmem.NewStore(), "test/log", true)
			if err != nil {
				t.Fatal(err)
			}

			// Keep eviction timers, work and cancellation inside synthetic time.
			synctest.Test(t, func(t *testing.T) {
				// Open a placement over in-memory stores without external services.
				zero := name == "zero"
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				remote := newLeafStore()
				p := &placementState{wb: wb, evictGate: new(csync.Mutex), remote: new(atomic.Pointer[openBackend])}
				if !zero {
					p.remote.Store(&openBackend{store: openedBackend{StoreOps: remote}})
				}
				wb.SetPolicy(block_store_writeback.Policy{ResidentHigh: 1})
				logger, hook := logrustest.NewNullLogger()
				// Count the minute summaries, not the pass start lines.
				summaries := func() []*logrus.Entry {
					var out []*logrus.Entry
					for _, entry := range hook.AllEntries() {
						if entry.Message == "eviction pass" {
							out = append(out, entry)
						}
					}
					return out
				}
				done := make(chan error, 1)
				go func() { done <- p.runEviction(ctx, logrus.NewEntry(logger)) }()

				// Fill actual eviction entries and let every runnable pass finish.
				fill := func(data string) {
					// Confirm the remote copy before making it resident locally.
					ref, _, err := remote.PutBlock(ctx, []byte(data), &block.PutOpts{RefsKnown: true})
					if err != nil {
						t.Fatal(err)
					}
					wb.Refill(ref, &block.StoredBlock{Data: []byte(data), RefsSource: block.RefsAsserted})
					synctest.Wait()
				}
				fill("first")
				if entries := summaries(); len(entries) != 0 {
					t.Fatalf("logged before the interval ended: %+v", entries)
				}

				// An unopened backend returns zero even across repeated shortfalls.
				if zero {
					time.Sleep(2 * evictShortWait)
					synctest.Wait()
					if status, _ := wb.GetStatus(); status.EvictableBytes != 5 {
						t.Fatalf("zero pass changed eviction bytes: %+v", status)
					}
				} else {
					// A second completed pass joins the first minute's totals.
					time.Sleep(evictLogInterval / 2)
					fill("second")
					if entries := summaries(); len(entries) != 0 {
						t.Fatalf("logged before the interval ended: %+v", entries)
					}

					// Hold the next pass across the first interval's deadline.
					release, err := p.evictGate.Lock(ctx)
					if err != nil {
						t.Fatal(err)
					}
					fill("third")
					time.Sleep(evictLogInterval / 2)
					synctest.Wait()
					entries := summaries()
					if len(entries) != 1 || entries[0].Data["evicted"] != 2 || entries[0].Data["evicted-bytes"] != int64(11) {
						t.Fatalf("first minute totals = %+v", entries)
					}

					// Complete the blocked pass; cancellation must flush its interval.
					release()
					synctest.Wait()
					if entries := summaries(); len(entries) != 1 {
						t.Fatalf("new interval logged early: %+v", entries)
					}
				}

				// Join the routine and check that shutdown flushes only real work.
				cancel()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("routine exit = %v, want canceled", err)
				}
				entries := summaries()
				if zero {
					if len(entries) != 0 {
						t.Fatalf("zero passes logged: %+v", entries)
					}
				} else if len(entries) != 2 || entries[1].Data["evicted"] != 1 || entries[1].Data["evicted-bytes"] != int64(5) {
					t.Fatalf("shutdown totals = %+v", entries)
				}
			})
		})
	}
}

// TestEvictionPassLogCounts preserves every counter when multiple passes join
// an interval, suppresses empty intervals, and flushes a partial result on stop.
func TestEvictionPassLogCounts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Start the same worker the provider joins on exit.
		logger, hook := logrustest.NewNullLogger()
		passes := make(chan block_store_writeback.EvictResult)
		done := make(chan struct{})
		go func() {
			defer close(done)
			logEvictionPasses(logrus.NewEntry(logger), passes)
		}()

		// Empty results leave the worker idle even after a minute.
		passes <- block_store_writeback.EvictResult{}
		time.Sleep(evictLogInterval)
		synctest.Wait()
		if entries := hook.AllEntries(); len(entries) != 0 {
			t.Fatalf("empty interval logged: %+v", entries)
		}

		// Add all four counts from several passes in the first active minute.
		passes <- block_store_writeback.EvictResult{Evicted: 1, EvictedBytes: 7, Requeued: 2, Refused: 3}
		time.Sleep(evictLogInterval / 2)
		passes <- block_store_writeback.EvictResult{Evicted: 4, EvictedBytes: 11, Requeued: 5, Refused: 6}
		time.Sleep(evictLogInterval / 2)
		synctest.Wait()
		entries := hook.AllEntries()
		want := logrus.Fields{"evicted": 5, "evicted-bytes": int64(18), "requeued": 7, "refused": 9}
		if len(entries) != 1 {
			t.Fatalf("logged %d intervals, want 1", len(entries))
		}
		for key, value := range want {
			if entries[0].Data[key] != value {
				t.Fatalf("interval counts = %v, want %v", entries[0].Data, want)
			}
		}

		// Closing the producer flushes a nonempty partial interval exactly once.
		passes <- block_store_writeback.EvictResult{Refused: 1}
		close(passes)
		<-done
		entries = hook.AllEntries()
		if len(entries) != 2 || entries[1].Data["refused"] != 1 {
			t.Fatalf("partial interval = %+v", entries)
		}
	})
}

// TestReadSetKeepsRecentReads checks that eviction keeps a read block across
// passes until two generations of readKeepAge have turned.
func TestReadSetKeepsRecentReads(t *testing.T) {
	// Record a read.
	ctx := t.Context()
	now := time.Now()
	r := readSet{clock: func() time.Time { return now }}
	r.enabled.Store(true)
	read, other := hash.NewHash(hash.HashType_HashType_SHA256, []byte("read")), hash.NewHash(hash.HashType_HashType_SHA256, []byte("other"))
	r.add(read)

	// Passes in quick succession keep it.
	take := func() func(*hash.Hash) bool {
		keep, err := r.take(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return keep
	}
	for range 3 {
		if keep := take(); !keep(read) || keep(other) {
			t.Fatal("pass did not keep exactly the read block")
		}
	}

	// One turn keeps it in the previous generation, the second drops it.
	now = now.Add(readKeepAge)
	if !take()(read) {
		t.Fatal("first turn dropped the read block")
	}
	now = now.Add(readKeepAge)
	if take()(read) {
		t.Fatal("second turn kept the read block")
	}

	// A generation idle for two ages drops both at once.
	r.add(read)
	take()
	now = now.Add(2 * readKeepAge)
	if take()(read) {
		t.Fatal("pass after two idle ages kept the read block")
	}
}

// TestReadSetKeepsReadsAcrossRestart checks that the saved reads keep their
// blocks after a restart, through one turn, and that the second turn drops
// them from the store too.
func TestReadSetKeepsReadsAcrossRestart(t *testing.T) {
	// Open a writeback store to save the reads to.
	ctx := t.Context()
	local := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0)
	wb, err := block_store_writeback.NewStore(ctx, local, local, store_kvtx_inmem.NewStore(), "test/markers", true)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	restart := func() *readSet {
		r := &readSet{clock: func() time.Time { return now }}
		r.enabled.Store(true)
		if err := r.load(ctx, wb); err != nil {
			t.Fatal(err)
		}
		return r
	}
	take := func(r *readSet) func(*hash.Hash) bool {
		keep, err := r.take(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return keep
	}

	// A read saved before a restart keeps its block after it.
	read, later, other := hash.NewHash(hash.HashType_HashType_SHA256, []byte("read")), hash.NewHash(hash.HashType_HashType_SHA256, []byte("later")), hash.NewHash(hash.HashType_HashType_SHA256, []byte("other"))
	r := restart()
	r.add(read)
	if err := r.save(ctx); err != nil {
		t.Fatal(err)
	}
	r = restart()
	if keep := take(r); !keep(read) || keep(other) {
		t.Fatal("restart did not keep exactly the saved read")
	}

	// After one turn and a restart, the previous generation still keeps it,
	// and a read since the turn is kept too.
	now = now.Add(readKeepAge)
	take(r)
	r.add(later)
	if err := r.save(ctx); err != nil {
		t.Fatal(err)
	}
	r = restart()
	if keep := take(r); !keep(read) || !keep(later) {
		t.Fatal("restart after a turn dropped a saved read")
	}

	// The second turn drops the first read, and the store forgets it.
	now = now.Add(readKeepAge)
	if keep := take(r); keep(read) || !keep(later) {
		t.Fatal("second turn did not drop only the first read")
	}
	r = restart()
	if keep := take(r); keep(read) || !keep(later) {
		t.Fatal("store kept the dropped generation")
	}
}

// TestEvictionKeepsCacheHits checks that a block served from a decoded-block
// cache is kept for readKeepAge on the hit set's own clock, without changing
// what the read set keeps: a hit never starts or turns its generations.
func TestEvictionKeepsCacheHits(t *testing.T) {
	// Start a read set and a hit set on one clock.
	ctx := t.Context()
	now := time.Now()
	clock := func() time.Time { return now }
	r := &readSet{clock: clock}
	r.enabled.Store(true)
	hits := &hitSet{clock: clock}
	newRef := func(name string) *block.BlockRef {
		return block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte(name)))
	}
	take := func() func(*hash.Hash) bool {
		keep, err := r.take(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return func(h *hash.Hash) bool { return keep(h) || hits.has(h) }
	}

	// A hit leaves the read set's clock alone: a block read from the store
	// 59 minutes after the hit is still kept by the pass an hour after that,
	// as it was without hits.
	cached, unread, stored := newRef("cached"), newRef("unread"), newRef("stored")
	hitAt := now
	hits.add(cached.GetHash())
	if !r.turned.IsZero() {
		t.Fatal("a cache hit started the read set's generation")
	}
	now = now.Add(59 * time.Minute)
	r.add(stored.GetHash())
	now = now.Add(61 * time.Minute)
	if keep := take(); !keep(stored.GetHash()) || keep(unread.GetHash()) {
		t.Fatal("pass did not keep the stored block alone")
	}

	// The hit is kept for at least readKeepAge on its own clock, and dropped
	// two generations later; a block never hit is not kept.
	now = hitAt.Add(readKeepAge)
	if !hits.has(cached.GetHash()) || hits.has(unread.GetHash()) {
		t.Fatal("hit set dropped the hit within readKeepAge or kept an unread block")
	}
	now = hitAt.Add(2 * readKeepAge)
	if hits.has(cached.GetHash()) {
		t.Fatal("hit set kept the hit past two generations")
	}
}

// TestEvictionResumesToLowMark checks that a pass the high mark started, cut
// short with the resident bytes back at the high mark, is carried on to the
// low mark by the next run of the eviction loop, as the routine's restart
// runs it, though the resident bytes no longer exceed the high mark: on be-7 a
// pass that ended 4.6 GB into its target at 01:14:33Z waited for the high mark
// to be crossed again.
func TestEvictionResumesToLowMark(t *testing.T) {
	// Batch ten blocks at a time, so a pass can stop after one batch.
	block_store_writeback.SetEvictBatch(block_store_writeback.EvictBatch{MinBlocks: 10, MaxBlocks: 10})
	t.Cleanup(func() { block_store_writeback.SetEvictBatch(block_store_writeback.EvictBatch{}) })

	// Write and upload 100 blocks of 1 KiB to the packs.
	f := newFallbackTest(t)
	const total, size = 100, 1 << 10
	for i := range total {
		data := make([]byte, size)
		copy(data, fmt.Appendf(nil, "block-%05d", i))
		if _, _, err := f.wb.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
	}
	f.upload()

	// Open the packs as the backend, and refuse the pass's second batch once.
	f.p.remote = &atomic.Pointer[openBackend]{}
	f.p.remote.Store(&openBackend{backend: f.p.backend.Load(), store: f.packs})
	f.p.evictGate, f.p.policyWake = &csync.Mutex{}, make(chan struct{}, 1)
	f.p.evictAdmit = &atomic.Pointer[block_store_writeback.EvictAdmit]{}
	stop := errors.New("paused")
	var batches atomic.Int32
	admit := block_store_writeback.EvictAdmit(func(context.Context, int64) error {
		if batches.Add(1) == 2 {
			return stop
		}
		return nil
	})
	f.p.evictAdmit.Store(&admit)

	// Put the store past a high mark of 90 blocks, to a low mark of 20.
	const high, low = 90 * size, 20 * size
	resident := func() int64 {
		status, _ := f.wb.GetStatus()
		return status.ResidentBytes()
	}
	if got := resident(); got != total*size {
		t.Fatalf("resident = %d before eviction, want %d", got, total*size)
	}
	f.wb.SetPolicy(block_store_writeback.Policy{ResidentHigh: high, ResidentLow: low})

	// The first run drops one batch, back to the high mark, and ends on the
	// refused batch.
	le := logrus.NewEntry(logrus.New())
	if err := f.p.runEviction(f.ctx, le); !errors.Is(err, stop) {
		t.Fatalf("first run = %v, want the refused batch", err)
	}
	if got := resident(); got != high {
		t.Fatalf("resident = %d after the cut-short pass, want the high mark %d", got, high)
	}

	// The next run carries the pass on to the low mark.
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- f.p.runEviction(ctx, le) }()
	deadline := time.After(5 * time.Second)
	for {
		status, changed := f.wb.GetStatus()
		if status.ResidentBytes() <= low {
			break
		}
		select {
		case <-changed:
		case err := <-done:
			t.Fatalf("eviction loop ended: %v", err)
		case <-deadline:
			cancel()
			t.Fatalf("resident = %d, want the loop to evict to the low mark %d", status.ResidentBytes(), low)
		}
	}
	cancel()
	<-done
	if got := resident(); got != low {
		t.Fatalf("resident = %d after the loop, want the low mark %d", got, low)
	}
}
