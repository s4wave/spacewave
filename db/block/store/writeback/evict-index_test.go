package block_store_writeback

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// newIndexTestStore joins the real store worker before deterministic slice
// and fault injection tests. The volume stays open under the test context.
func newIndexTestStore(t *testing.T, env *testEnv) *Store {
	// Open the store with a separately owned repair lifetime.
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	s, err := NewStore(ctx, ownedStore{env.vol}, env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	// Stop the worker before changing wrappers or driving explicit slices.
	cancel()
	<-s.indexDone
	return s
}

// checkEvictIndex verifies the durable byte sum and canonical entries.
func checkEvictIndex(t *testing.T, s *Store, wantEntries int, wantBytes int64) {
	// Compare the stored projection with the exact metadata entries.
	t.Helper()
	var count int
	var total int64
	err := kvtx.RunTransaction(t.Context(), false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		err := scanEvictable(ctx, tx, nil, func(entry evictEntry) error {
			count++
			total += entry.size
			return nil
		})
		if err != nil {
			return err
		}
		durable, err := readEvictableBytes(ctx, tx)
		if err == nil && durable != total {
			t.Errorf("durable bytes=%d entries sum=%d", durable, total)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Status deltas must also match, including a replayed repair slice.
	status, _ := s.GetStatus()
	if count != wantEntries || total != wantBytes || status.EvictableBytes != total {
		t.Fatalf("entries=%d bytes=%d status=%d; want %d/%d", count, total, status.EvictableBytes, wantEntries, wantBytes)
	}
}

// TestEvictIndexRebuildRestart repairs legacy duplicates and marked overlaps
// across slices, a failed commit and a reopen, without touching local values.
func TestEvictIndexRebuildRestart(t *testing.T) {
	// Seed more than one slice through real legacy producers in two epochs.
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	s := newIndexTestStore(t, env)
	batch := make([]fill, 0, evictRetryCommitBlocks+3)
	for i := range evictRetryCommitBlocks + 3 {
		data := fmt.Appendf(nil, "legacy-index-%03d", i)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, fill{ref: ref, stored: &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}})
	}
	if err := s.writeFills(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Evict(ctx, newInmemBlockStore(), 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PutBlock(ctx, batch[0].stored.Data, leafOpts); err != nil {
		t.Fatal(err)
	}

	// Enable the index and interrupt a repair after exactly one bounded slice.
	SetEvictQueueIndex(true)
	background := block.WithBackground(ctx)
	next, done, err := s.repairEvictIndexSlice(background, nil, 10)
	if err != nil || done || next == nil {
		t.Fatalf("first repair slice=%q done=%v err=%v", next, done, err)
	}
	failure := errors.New("index repair commit failed")
	original := s.markers
	s.markers = &evictCommitStore{Store: original, err: failure}
	if _, _, err := s.repairEvictIndexSlice(background, next, 10); !errors.Is(err, failure) {
		t.Fatalf("repair failure=%v", err)
	}
	s.markers = original
	env.open(t)
	s = newIndexTestStore(t, env)

	// Restart from zero and repeat the finished walk: bytes cannot decrement twice.
	for range 2 {
		var from []byte
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
	want := int64(0)
	for _, f := range batch[1:] {
		want += int64(len(f.stored.Data))
	}
	checkEvictIndex(t, s, len(batch)-1, want)

	// Metadata correction must preserve every byte and must never consult remote.
	for _, f := range batch {
		got, found, err := s.values.GetBlock(ctx, f.ref)
		if err != nil || !found || !bytes.Equal(got, f.stored.Data) {
			t.Fatalf("repair altered value=%q found=%v err=%v", got, found, err)
		}
	}
}

// TestEvictIndexDroppedClearSwitches keeps marker-covered fills reachable when
// either opt-in is disabled after reservation, then repairs legacy duplicates
// on re-enable. A never-indexed retry-off marker retains the legacy behavior.
func TestEvictIndexDroppedClearSwitches(t *testing.T) {
	for _, disable := range []string{"index", "retry", "both", "legacy"} {
		t.Run(disable, func(t *testing.T) {
			// Upload, mark again, and let the next upload capture an absent value.
			SetEvictRefusedRetry(true)
			SetEvictQueueIndex(false)
			t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
			ctx := t.Context()
			env := newTestEnv(t)
			s := newIndexTestStore(t, env)
			if disable != "legacy" {
				SetEvictQueueIndex(true)
			}
			remote := newInmemBlockStore()
			data := []byte("marked fill across policy disable")
			ref, _, err := s.PutBlock(ctx, data, leafOpts)
			if err != nil {
				t.Fatal(err)
			}
			drain(t, s, remote)
			if _, _, err := s.PutBlock(ctx, data, leafOpts); err != nil {
				t.Fatal(err)
			}
			if err := s.local.RmBlock(ctx, ref); err != nil {
				t.Fatal(err)
			}
			head, err := s.scanHead(ctx, batchLimitsOf(nil), nil)
			if err != nil || len(head.entries) != 1 {
				t.Fatalf("head=%v err=%v", head, err)
			}
			if err := s.uploadChunk(ctx, remote, head.entries); err != nil || !head.entries[0].dropped {
				t.Fatalf("absent upload=%v", err)
			}

			// Restore the value while its marker covers it, then disable policy.
			if err := s.writeFills(ctx, []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}}}); err != nil {
				t.Fatal(err)
			}
			if disable != "legacy" {
				checkEvictIndex(t, s, 0, 0)
			}
			if disable == "index" || disable == "both" {
				SetEvictQueueIndex(false)
			}
			if disable != "index" {
				SetEvictRefusedRetry(false)
			}
			if err := s.clear(ctx, head.entries); err != nil {
				t.Fatal(err)
			}
			if disable != "legacy" {
				checkEvictIndex(t, s, 1, int64(len(data)))
			}

			// Missing remote data must still prevent deletion after a switch flip.
			SetEvictRefusedRetry(true)
			SetEvictQueueIndex(true)
			if err := remote.RmBlock(ctx, ref); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Evict(ctx, remote, 1<<30, nil, nil); err != nil {
				t.Fatal(err)
			}
			if got, found, err := s.values.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
				t.Fatalf("last copy after switch=%q found=%v err=%v", got, found, err)
			}
		})
	}
}

// TestEvictIndexStaleSnapshot preserves a newer pointer and exact byte total
// when eviction commits metadata from an entry that clear/fill already moved.
func TestEvictIndexStaleSnapshot(t *testing.T) {
	// Queue one indexed entry, then move it after taking the old scan result.
	SetEvictRefusedRetry(true)
	SetEvictQueueIndex(false)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	s := newIndexTestStore(t, newTestEnv(t))
	SetEvictQueueIndex(true)
	data := []byte("move after snapshot")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		n, err := queueEvict(ctx, tx, 0, ref.GetHash(), int64(len(data)))
		if err != nil {
			return err
		}
		return addEvictableBytes(ctx, tx, n)
	})
	if err != nil {
		t.Fatal(err)
	}
	s.addEvictStatus(int64(len(data)), 0)

	// Delete the obsolete key twice. Neither deletion may subtract or clear
	// the move.
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		n, err := queueEvict(ctx, tx, 1, ref.GetHash(), int64(len(data)))
		if err != nil {
			return err
		}
		for range 2 {
			removed, _, err := deleteEvictEntry(ctx, tx, evictEntry{key: evictKey(0, ref.GetHash()), hash: ref.GetHash(), size: int64(len(data))})
			if err != nil || removed != 0 {
				return errors.New("stale deletion subtracted bytes")
			}
		}
		pointer, found, err := tx.Get(ctx, evictIndexKey(ref.GetHash()))
		if err != nil || !found || !bytes.Equal(pointer, evictKey(1, ref.GetHash())) {
			return errors.New("stale deletion removed current pointer")
		}
		return addEvictableBytes(ctx, tx, n)
	})
	if err != nil {
		t.Fatal(err)
	}
	checkEvictIndex(t, s, 1, int64(len(data)))
}

// TestEvictIndexWorkerLifecycle repairs after runtime enable, invalidates a
// completed walk after off-policy writes, and joins when its owner stops.
func TestEvictIndexWorkerLifecycle(t *testing.T) {
	// Open the worker under an explicit lifetime and seed legacy duplicate rows.
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	workerCtx, cancel := context.WithCancel(ctx)
	s, err := NewStore(workerCtx, ownedStore{env.vol}, env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); <-s.indexDone }()
	data := []byte("runtime repair")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	fills := []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}}}
	for range 2 {
		if err := s.writeFills(ctx, fills); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Evict(ctx, newInmemBlockStore(), 0, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	checkEvictIndex(t, s, 2, 2*int64(len(data)))

	// A runtime enable repairs without another request driving the rebuild.
	SetEvictQueueIndex(true)
	waitStatus(t, s, func(st Status) bool { return st.EvictableBytes == int64(len(data)) })
	checkEvictIndex(t, s, 1, int64(len(data)))

	// Legacy mutation invalidates the completed walk. Enabling starts from
	// zero.
	SetEvictQueueIndex(false)
	if _, err := s.Evict(ctx, newInmemBlockStore(), 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}
	SetEvictQueueIndex(true)
	waitStatus(t, s, func(st Status) bool { return st.EvictableBytes == int64(len(data)) })
	checkEvictIndex(t, s, 1, int64(len(data)))
}

// TestEvictIndexMarkerAndClearFailures keeps marker coverage, byte accounting
// and local values together when either metadata transaction is discarded.
func TestEvictIndexMarkerAndClearFailures(t *testing.T) {
	// Index a remotely held local fill before injecting marker failures.
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	s := newIndexTestStore(t, env)
	SetEvictQueueIndex(true)
	remote := newInmemBlockStore()
	data := []byte("discard metadata without discarding values")
	ref, _, err := remote.PutBlock(ctx, data, leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}}}); err != nil {
		t.Fatal(err)
	}
	marks := []Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}
	original := s.markers
	failure := errors.New("discard indexed metadata")
	s.markers = &evictCommitStore{Store: original, err: failure}

	// A failed marker transaction must leave the original indexed bytes alone.
	if err := s.queue(ctx, marks); !errors.Is(err, failure) {
		t.Fatalf("marker failure=%v", err)
	}
	checkEvictIndex(t, s, 1, int64(len(data)))
	if env.isMarked(t, ref) {
		t.Fatal("discarded marker was published")
	}
	s.markers = original
	if err := s.queue(ctx, marks); err != nil {
		t.Fatal(err)
	}
	checkEvictIndex(t, s, 0, 0)

	// A failed clear must retain the marker and its coverage reservation.
	head, err := s.scanHead(ctx, batchLimitsOf(nil), nil)
	if err != nil || len(head.entries) != 1 {
		t.Fatalf("head=%v err=%v", head, err)
	}
	s.markers = &evictCommitStore{Store: original, err: failure}
	if err := s.clear(ctx, head.entries); !errors.Is(err, failure) {
		t.Fatalf("clear failure=%v", err)
	}
	checkEvictIndex(t, s, 0, 0)
	if !env.isMarked(t, ref) {
		t.Fatal("failed clear lost its marker")
	}
	s.markers = original
	if err := s.clear(ctx, head.entries); err != nil {
		t.Fatal(err)
	}
	checkEvictIndex(t, s, 1, int64(len(data)))
	if got, found, err := s.values.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("metadata failure changed local value=%q found=%v err=%v", got, found, err)
	}
}

// TestEvictIndexQueuedRepairDoesNotHoldDisable pauses before writer admission
// and proves disable/reset completes without waiting for background repair.
func TestEvictIndexQueuedRepairDoesNotHoldDisable(t *testing.T) {
	// Seed duplicate legacy metadata while keeping the local value intact.
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	s := newIndexTestStore(t, env)
	data := []byte("disable while repair waits")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	fills := []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}}}
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

	// Install a canonical pointer so reset must remove the new key family too.
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}

	// Pause a background request after its generation snapshot.
	delayed := &evictAdmissionStore{Store: s.markers, entered: make(chan struct{}), release: make(chan struct{})}
	s.markers = delayed
	repairCtx, cancel := context.WithCancel(ctx)
	repaired := make(chan error, 1)
	repairDone := make(chan struct{})
	go func() {
		defer close(repairDone)
		_, _, err := s.repairEvictIndexSlice(block.WithBackground(repairCtx), nil, 2)
		repaired <- err
	}()
	defer func() { cancel(); <-repairDone }()
	<-delayed.entered

	// Disable is foreground work and must not wait for that unadmitted request.
	disableCtx, stopDisable := context.WithTimeout(ctx, time.Second)
	defer stopDisable()
	if err := s.SetEnabled(disableCtx, false); err != nil {
		t.Fatalf("disable waited behind repair admission: %v", err)
	}
	checkEvictIndex(t, s, 0, 0)
	err = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// The index pointer must not outlive its queue's reset.
		found, err := tx.Exists(ctx, evictIndexKey(ref.GetHash()))
		if err == nil && found {
			t.Error("reset retained an eviction index pointer")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	close(delayed.release)
	if err := <-repaired; err != nil {
		t.Fatal(err)
	}

	// A stale committed projection cannot subtract from the reset generation.
	if err := s.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	checkEvictIndex(t, s, 0, 0)
	if got, found, err := s.values.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("repair/disable changed value=%q found=%v err=%v", got, found, err)
	}
}

// TestEvictIndexInitiallyDisabledRepairs corrects restored duplicate bytes
// without enabling uploads or ignoring their nonzero eviction projection.
func TestEvictIndexInitiallyDisabledRepairs(t *testing.T) {
	// Seed legacy entries through fills at two different epochs.
	SetEvictQueueIndex(false)
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictQueueIndex(false); SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	s := newIndexTestStore(t, env)
	data := []byte("disabled restored accounting")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	fills := []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}}}
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Evict(ctx, newInmemBlockStore(), 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.writeFills(ctx, fills); err != nil {
		t.Fatal(err)
	}

	// Open with upload disabled and let the actual context-owned worker repair.
	SetEvictQueueIndex(true)
	workerCtx, cancel := context.WithCancel(ctx)
	s, err = NewStore(workerCtx, ownedStore{env.vol}, env.vol, env.markers(t), testMarkersID, false)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); <-s.indexDone }()
	waitStatus(t, s, func(st Status) bool { return st.EvictableBytes == int64(len(data)) })
	checkEvictIndex(t, s, 1, int64(len(data)))
	if status, _ := s.GetStatus(); status.Enabled {
		t.Fatal("repair enabled upload")
	}
}
