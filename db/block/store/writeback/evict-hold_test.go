//go:build !js && !wasip1

package block_store_writeback

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// evictHoldStore measures queue-changing transactions on the native engine.
type evictHoldStore struct {
	kvtx.Store
	// holds contains the writer hold of each successful queue transaction.
	holds []time.Duration
	// entries counts the old entries deleted by successful transactions.
	entries int
	// probeCtx enables one foreground admission request per measured slice.
	probeCtx context.Context
	// writerQueue observes the native foreground queue while the slice holds it.
	writerQueue kvtx.WriterQueueStore
	// probes records joined requests; pure hold measurements leave probeCtx nil.
	probes []evictWriterProbe
}

// NewTransaction starts timing after the engine has acquired the writer.
func (s *evictHoldStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Leave reads alone and time writes after writer admission.
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	// Keep controlled probe overhead separate from the pure-hold arm.
	measured := &evictHoldTx{Tx: tx, store: s, start: time.Now()}
	if s.probeCtx != nil && block.IsBackground(ctx) {
		ready := make(chan struct{})
		probe := make(chan evictWriterProbe, 1)
		measured.probe = probe
		go func() {
			before := time.Now()
			close(ready)
			writer, err := s.Store.NewTransaction(s.probeCtx, true)
			result := evictWriterProbe{started: before, wait: time.Since(before), err: err}
			if err == nil {
				writer.Discard()
			}
			probe <- result
		}()
		<-ready

		// Observe actual native registration before releasing this background
		// writer. The goroutine's start signal alone cannot prove admission.
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for !s.writerQueue.WritersWaiting() {
			select {
			case <-ctx.Done():
				tx.Discard()
				return nil, ctx.Err()
			case <-deadline.C:
				tx.Discard()
				return nil, fmt.Errorf("foreground probe never entered native writer queue")
			default:
				runtime.Gosched()
			}
		}
	}
	return measured, nil
}

// evictHoldTx tracks the queue changes made while holding the native writer.
type evictHoldTx struct {
	kvtx.Tx
	// store receives the completed transaction's measurement.
	store *evictHoldStore
	// start is recorded after NewTransaction acquired the writer.
	start time.Time
	// probe is the controlled foreground request joined after writer release.
	probe <-chan evictWriterProbe
	// queued records marker-queue writes in either foreground measurement arm.
	queued bool
	// indexed records a hash-index write, including pointer-only repair.
	indexed bool
	// entries counts old eviction entries removed by this transaction.
	entries int
}

// Delete counts eviction entries while forwarding the real engine write.
func (t *evictHoldTx) Delete(ctx context.Context, key []byte) error {
	if bytes.HasPrefix(key, []byte(evictPrefix)) {
		t.entries++
	}
	return t.Tx.Delete(ctx, key)
}

// Set observes pointer-only repair transactions while forwarding native writes.
func (t *evictHoldTx) Set(ctx context.Context, key, value []byte) error {
	t.indexed = t.indexed || bytes.HasPrefix(key, []byte(evictIndexPrefix))
	t.queued = t.queued || bytes.HasPrefix(key, []byte(queuePrefix))
	return t.Tx.Set(ctx, key, value)
}

// Commit measures through writer release; ordered eviction commits avoid fsync.
func (t *evictHoldTx) Commit(ctx context.Context) error {
	// Record only successful queue transactions, excluding the epoch advance.
	err := t.Tx.Commit(ctx)
	if err == nil && (t.entries != 0 || t.indexed || t.queued) {
		t.store.holds = append(t.store.holds, time.Since(t.start))
		t.store.entries += t.entries
	}
	if err == nil {
		t.joinProbe()
	}
	return err
}

// Discard releases the writer before joining a controlled foreground request.
func (t *evictHoldTx) Discard() {
	t.Tx.Discard()
	t.joinProbe()
}

// joinProbe records only a request started before this writer was released.
func (t *evictHoldTx) joinProbe() {
	if t.probe != nil {
		released := time.Now()
		result := <-t.probe
		result.overlap = result.started.Before(released)
		t.store.probes = append(t.store.probes, result)
		t.probe = nil
	}
}

// evictWriterProbe records one controlled native foreground admission.
type evictWriterProbe struct {
	// started is captured while the background writer still owns admission.
	started time.Time
	// wait includes native writer admission, scheduling and engine I/O.
	wait time.Duration
	// overlap reports that the request began before the writer released.
	overlap bool
	// err is any native admission failure.
	err error
}

// TestEvictRefusedWriterHold measures one 1,024-refusal pass per policy on
// an on-disk native Volume. Disabling optimistic transactions measures the
// whole writer hold, including the fallback used after repeated conflicts.
func TestEvictRefusedWriterHold(t *testing.T) {
	// Make the transaction body part of the measured writer hold in both arms.
	t.Setenv(kvtx.OptimisticEnv, "off")
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%t", retry), func(t *testing.T) {
			// Open the same s4db Volume used by native provider storage.
			SetEvictRefusedRetry(retry)
			t.Cleanup(func() { SetEvictRefusedRetry(false) })
			ctx := t.Context()
			vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{
				Path: filepath.Join(t.TempDir(), "evict.s4wave"),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = vol.Close() })
			markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			measured := &evictHoldStore{Store: markers, writerQueue: volume_s4db.GetDB(vol)}
			s, err := NewStore(ctx, ownedStore{vol}, vol, measured, testMarkersID, true)
			if err != nil {
				t.Fatal(err)
			}

			// Queue a full scan batch of unmarked local fills absent remotely.
			for i := range evictBatchStartBlocks {
				data := fmt.Appendf(nil, "writer-hold-refusal-%05d", i)
				ref, err := block.BuildBlockRef(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				s.Refill(ref, &block.StoredBlock{Data: data, RefsKnown: true})
			}
			waitStatus(t, s, func(st Status) bool { return st.Refills == evictBatchStartBlocks })

			// Time only the actual queue transactions, excluding scan and probes.
			result, err := s.Evict(ctx, newInmemBlockStore(), 1<<30, nil, nil)
			if err != nil || result != (EvictResult{Refused: evictBatchStartBlocks}) || measured.entries != evictBatchStartBlocks {
				t.Fatalf("measured pass = %+v, %v, %d entries", result, err, measured.entries)
			}
			wantCommits := 1
			if retry {
				wantCommits = (evictBatchStartBlocks + evictRetryCommitBlocks - 1) / evictRetryCommitBlocks
			}
			if len(measured.holds) != wantCommits {
				t.Fatalf("queue transactions = %d, want %d", len(measured.holds), wantCommits)
			}
			var total, longest time.Duration
			for _, hold := range measured.holds {
				total += hold
				longest = max(longest, hold)
			}
			t.Logf("retry=%t entries=%d transactions=%d writer-total=%s writer-max=%s", retry, measured.entries, len(measured.holds), total, longest)
		})
	}
}

// TestEvictIndexWriterHold measures native rebuild slices and foreground marker
// accounting on real s4db storage with optimistic fallback's full writer hold.
func TestEvictIndexWriterHold(t *testing.T) {
	// Keep repairs explicit and prevent the store worker from racing measurements.
	t.Setenv(kvtx.OptimisticEnv, "off")
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "index.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	measured := &evictHoldStore{Store: markers, writerQueue: volume_s4db.GetDB(vol)}
	workerCtx, cancel := context.WithCancel(ctx)
	s, err := NewStore(workerCtx, ownedStore{vol}, vol, measured, testMarkersID, true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	<-s.indexDone

	// Seed two legacy epochs, then remove duplicates in bounded native slices.
	fills := make([]fill, 0, evictBatchStartBlocks)
	marks := make([]Mark, 0, 2*evictRetryCommitBlocks)
	for i := range evictBatchStartBlocks {
		data := fmt.Appendf(nil, "index-hold-%05d", i)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		fills = append(fills, fill{ref: ref, stored: &block.StoredBlock{Data: data, RefsKnown: true}})
		if len(marks) < 2*evictRetryCommitBlocks {
			marks = append(marks, Mark{Hash: ref.GetHash(), Size: int64(len(data))})
		}
	}
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Evict(ctx, newInmemBlockStore(), 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}
	SetEvictQueueIndex(true)

	// Measure every pointer-only and duplicate-removal slice without probes.
	repair := func() {
		var from []byte
		background := block.WithBackground(kvtx.WithOrderedCommitContext(ctx))
		for {
			next, done, err := s.repairEvictIndexSlice(background, from, 10)
			if err != nil {
				t.Fatal(err)
			}
			if done {
				break
			}
			from = next
		}
	}
	repair()
	var total, longest time.Duration
	for _, hold := range measured.holds {
		total += hold
		longest = max(longest, hold)
	}
	t.Logf("index-rebuild entries=%d transactions=%d writer-total=%s writer-max=%s", measured.entries, len(measured.holds), total, longest)
	if measured.entries != evictBatchStartBlocks || len(measured.holds) != 2*evictBatchStartBlocks/evictRetryCommitBlocks {
		t.Fatalf("rebuild deleted %d entries, want %d", measured.entries, evictBatchStartBlocks)
	}

	// Recreate legacy rows outside measurement, then start one real foreground
	// request under each admitted writer. The controlled arm includes rendezvous
	// overhead; its admission observations are separate from pure slice holds.
	SetEvictQueueIndex(false)
	if _, err := s.Evict(ctx, newInmemBlockStore(), 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}
	SetEvictQueueIndex(true)
	measured.holds, measured.entries, measured.probes = nil, 0, nil
	measured.probeCtx = ctx
	repair()
	measured.probeCtx = nil
	var wait time.Duration
	for _, probe := range measured.probes {
		if probe.err != nil || !probe.overlap {
			t.Fatalf("foreground probe overlap=%v err=%v", probe.overlap, probe.err)
		}
		wait = max(wait, probe.wait)
	}
	if len(measured.probes) != len(measured.holds) || len(measured.probes) != 2*evictBatchStartBlocks/evictRetryCommitBlocks {
		t.Fatalf("probe coverage=%d measured slices=%d", len(measured.probes), len(measured.holds))
	}
	t.Logf("index-rebuild-controlled probes=%d native-queued=%d foreground-admission-max=%s; includes per-slice rendezvous", len(measured.probes), len(measured.probes), wait)

	// Compare new-marker transactions for equally sized disjoint hash batches.
	// Both arms use full foreground durability; shared disk scheduling and fsync
	// variation mean the difference is an observation, not a causal estimate.
	SetEvictQueueIndex(false)
	measured.holds, measured.entries = nil, 0
	if err := s.queue(ctx, marks[:evictRetryCommitBlocks]); err != nil {
		t.Fatal(err)
	}
	if len(measured.holds) != 1 || measured.entries != 0 {
		t.Fatalf("legacy marker transactions=%d entries=%d", len(measured.holds), measured.entries)
	}
	baseline := measured.holds[0]
	SetEvictQueueIndex(true)
	measured.holds, measured.entries = nil, 0
	if err := s.queue(ctx, marks[evictRetryCommitBlocks:]); err != nil {
		t.Fatal(err)
	}
	if len(measured.holds) != 1 || measured.entries != evictRetryCommitBlocks {
		t.Fatalf("indexed marker transactions=%d entries=%d", len(measured.holds), measured.entries)
	}
	t.Logf("index-marker entries=%d transactions=1 legacy-writer-hold=%s indexed-writer-hold=%s observed-difference=%s", evictRetryCommitBlocks, baseline, measured.holds[0], measured.holds[0]-baseline)
}

// TestEvictIndexActivationFence commits an off-policy marker staged before
// the enabling repair's epoch fence on the real native optimistic engine
// without a conflict, covering its block under the policy at the commit.
func TestEvictIndexActivationFence(t *testing.T) {
	// Open native storage without a live repair worker to control the fence.
	t.Setenv(kvtx.OptimisticEnv, "on")
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "activation.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	workerCtx, cancel := context.WithCancel(ctx)
	s, err := NewStore(workerCtx, ownedStore{vol}, vol, markers, testMarkersID, true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	<-s.indexDone
	data := []byte("off-policy optimistic marker")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsKnown: true}}}); err != nil {
		t.Fatal(err)
	}

	// Stage the marker while off, then activate and commit the epoch fence.
	var delta *queueDelta
	var attempts int
	err = kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		attempts++
		var err error
		delta, err = queueMarks(ctx, tx, []Mark{{Hash: ref.GetHash(), Size: int64(len(data))}})
		if err != nil || attempts != 1 {
			return err
		}
		SetEvictQueueIndex(true)
		return kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, fence kvtx.Tx) error {
			// Install the canonical pointer just as the enabling repair would.
			if err := fence.Set(ctx, []byte(epochKey), []byte("1")); err != nil {
				return err
			}
			return fence.Set(ctx, evictIndexKey(ref.GetHash()), evictKey(0, ref.GetHash()))
		})
	}, kvtx.RetryInvalidSnapshot)
	if err != nil {
		t.Fatal(err)
	}

	// The commit must remove overlap under the new policy without a replay,
	// never publish stale bytes.
	if attempts != 1 {
		t.Fatalf("marker attempts=%d, want one with no conflict", attempts)
	}
	gen, _ := s.getEnabledGen()
	s.applyQueued(gen, *delta)
	checkEvictIndex(t, s, 0, 0)
	err = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		found, err := tx.Exists(ctx, markedKey(ref.GetHash()))
		if err == nil && !found {
			t.Error("replayed marker missing")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
