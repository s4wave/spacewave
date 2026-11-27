package provider_spacewave

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// TestSyncControllerHeldWaitIgnoresFlushBroadcast checks that a held store
// waits for a new write or its recheck deadline, not for the broadcast its own
// flush raised, so it does no flush while idle. A write of the held hash
// clears it from the known set.
func TestSyncControllerHeldWaitIgnoresFlushBroadcast(t *testing.T) {
	// Hold the only dirty block as corrupt.
	s := newDirtySyncExecuteTestController(t, nil, nil)
	ref, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: s.upper, bad: ref.GetHash()}
	writes, _ := s.writeSnapshot()
	var held *heldError
	if err := s.FlushNow(t.Context()); !errors.As(err, &held) {
		t.Fatalf("flush = %v; want held", err)
	}

	// The flush's own broadcast and any later one leave the wait to its
	// deadline, which reports a recheck.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	start := time.Now()
	recheck, err := s.waitHeld(t.Context(), writes, start.Add(200*time.Millisecond), time.Hour)
	if err != nil || !recheck || time.Since(start) < 200*time.Millisecond {
		t.Fatalf("wait = %v, %v after %v; want a recheck at the deadline", recheck, err, time.Since(start))
	}

	// A write ends the wait one interval later, without a recheck, and
	// clears the written hash from the known set.
	writes, _ = s.writeSnapshot()
	go func() {
		time.Sleep(20 * time.Millisecond)
		if err := s.MarkDirty(context.Background(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
			t.Error(err)
		}
	}()
	recheck, err = s.waitHeld(t.Context(), writes, time.Now().Add(time.Hour), 10*time.Millisecond)
	if err != nil || recheck {
		t.Fatalf("wait after write = %v, %v; want a flush without a recheck", recheck, err)
	}
	if hasKey(s.hold.blocks, ref.GetHash().MarshalString()) {
		t.Fatal("write of the held hash left it known corrupt")
	}
}

// TestSyncControllerHeldWriteDuringFlushRereads writes a held hash, after the
// block was removed and written again, while a flush is paused past its copy
// of the known set. That flush read nothing and holds the block, but the next
// flush must read it, upload it and clear it, not wait for the recheck
// deadline.
func TestSyncControllerHeldWriteDuringFlushRereads(t *testing.T) {
	// Accept every push.
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			uploads.Add(1)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, server.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Hold the only dirty block as corrupt.
	s := newDirtySyncExecuteTestController(t, cli, nil)
	good := s.upper
	ref, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: good, bad: ref.GetHash()}
	store := &pausingReadStore{Store: s.store, paused: make(chan struct{}), release: make(chan struct{})}
	s.store = store
	var held *heldError
	if err := s.FlushNow(t.Context()); !errors.As(err, &held) {
		t.Fatalf("first flush = %v; want held", err)
	}

	// Pause the next flush at its first page read, after its copy of the
	// known set, and rewrite the block meanwhile.
	store.pauseAt.Store(store.reads.Load() + 2)
	flushed := make(chan error, 1)
	go func() { flushed <- s.FlushNow(t.Context()) }()
	select {
	case <-store.paused:
	case err := <-flushed:
		t.Fatalf("flush ended before its pause: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("flush never reached its pause")
	}
	if err := good.RmBlock(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := good.PutBlock(t.Context(), []byte("dirty block"), nil); err != nil {
		t.Fatal(err)
	}
	s.upper = good
	if err := s.MarkDirty(t.Context(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	close(store.release)
	if err := <-flushed; !errors.As(err, &held) {
		t.Fatalf("paused flush = %v; want held from its copy", err)
	}
	if uploads.Load() != 0 {
		t.Fatalf("uploads before the rewrite was read = %d, want 0", uploads.Load())
	}

	// The next flush reads the rewritten block, uploads it and clears it.
	if err := s.FlushNow(t.Context()); err != nil {
		t.Fatalf("flush after the write = %v; want the block read and uploaded", err)
	}
	if uploads.Load() == 0 {
		t.Fatal("the rewritten block was not uploaded")
	}
	if pending, err := s.scanDirtyCandidates(t.Context()); err != nil || len(pending) != 0 {
		t.Fatalf("dirty after the flush = %d blocks, %v; want none", len(pending), err)
	}
}

// TestSyncControllerPartialPassKeepsHeld fails the push of a flush's first
// chunk, so the flush stops before reading every dirty block. The blocks it
// found corrupt are held with the count shown partial, a hold on a block it
// did not reach is kept, and a later complete flush replaces the set and
// clears partial. The flushes are unordered so the chunks follow the dirty
// sequence.
func TestSyncControllerPartialPassKeepsHeld(t *testing.T) {
	// Queue a late block behind the chunks and read the dirty block corrupt.
	ctx := t.Context()
	s, failFrom, pushes, upper := heldTestController(t)
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	late := addSyncDirtyBlock(t, ctx, wtx, upper.StoreOps, "late block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	s.updateDirtyState(ctx)
	bad, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey, lateKey := bad.GetHash().MarshalString(), late.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}

	// The first held flush fails its first push: held, partial, at least
	// the block it found.
	failFrom.Store(1)
	var held *heldError
	if err := s.FlushNowUnordered(ctx); err == nil || errors.As(err, &held) {
		t.Fatalf("failing flush = %v; want the push error", err)
	}
	wantHold(t, "after a failed push", s, []string{badKey}, true)
	var since time.Time
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { since = s.hold.since })

	// An earlier hold on the late block survives a rechecking flush that
	// fails before reaching it, with the first held time.
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.setHold(map[string]struct{}{badKey: {}, lateKey: {}}, false)
	})
	s.corruptRecheck.Store(true)
	pushes.Store(0)
	if err := s.FlushNowUnordered(ctx); err == nil || errors.As(err, &held) {
		t.Fatalf("second failing flush = %v; want the push error", err)
	}
	wantHold(t, "after a partial pass", s, []string{badKey, lateKey}, true)
	var since2 time.Time
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { since2 = s.hold.since })
	if !s.corruptRecheck.Load() || !since2.Equal(since) {
		t.Fatal("the partial pass lost the recheck or restarted the held time")
	}

	// A complete flush that reads every block replaces the set and clears
	// partial: the late block reads sound and uploads.
	failFrom.Store(0)
	if err := s.FlushNowUnordered(ctx); !errors.As(err, &held) || held.count != 1 {
		t.Fatalf("complete flush = %v; want held with exactly 1 block", err)
	}
	wantHold(t, "after a complete pass", s, []string{badKey}, false)
}

// TestSyncControllerWriteClearsPartialHold holds a block on a flush that
// fails its first push, then writes the block sound. The write empties the
// held set, so the store is not held and the status shows nothing held, and a
// flush that fails again before finding anything leaves the store unheld and
// the status showing nothing held.
func TestSyncControllerWriteClearsPartialHold(t *testing.T) {
	// Hold the dirty block on a failed flush.
	ctx := t.Context()
	s, failFrom, _, upper := heldTestController(t)
	bad, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey := bad.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}
	failFrom.Store(1)
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantHold(t, "after a failed push", s, []string{badKey}, true)

	// Remove the block and write it sound: nothing is held.
	delete(upper.bad, badKey)
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: bad.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	wantHold(t, "after the write", s, nil, false)

	// A flush that fails before finding anything keeps the store unheld.
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantHold(t, "after another failed push", s, nil, false)
}

// TestSyncControllerPartialPassDropsSoundHeld holds a block that has since
// read sound, with its recheck due, and damages a block in the same first
// chunk. The flush reads both, pushes and commits that chunk, then fails the
// next push. The block it proved sound must not be counted as held. The
// damaged block stays held, partial, with its marker and bytes.
func TestSyncControllerPartialPassDropsSoundHeld(t *testing.T) {
	// Hold the dirty block, sound on disk, with a recheck due, and damage the
	// first chunk-sized block.
	ctx := t.Context()
	s, failFrom, _, upper := heldTestController(t)
	sound, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := block.BuildBlockRef(bytes.Repeat([]byte{1}, syncTestChunkBlockBytes), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey := bad.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.setHold(map[string]struct{}{sound.GetHash().MarshalString(): {}}, false)
	})
	s.corruptRecheck.Store(true)

	// The second push fails after the first chunk committed.
	failFrom.Store(2)
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantHold(t, "after the partial pass", s, []string{badKey}, true)
	wantMarker(t, "after the partial pass", s, upper, bad)
}

// TestSyncControllerHeldMarkerSurvives tries to lose a damaged block's dirty
// marker across the paths that change the held state: a failed flush, a
// write of the damaged hash that keeps its damaged bytes and empties the
// held set, a write of another block, and a complete flush. The marker and
// the bytes must stay, and the last flush must hold exactly that block.
// TestSyncControllerPartialPassDropsSoundHeld covers a flush that fails after
// a committed chunk.
func TestSyncControllerHeldMarkerSurvives(t *testing.T) {
	// Read the dirty block as damaged throughout.
	ctx := t.Context()
	s, failFrom, _, upper := heldTestController(t)
	bad, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey := bad.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}

	// A failed flush holds it.
	failFrom.Store(1)
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantMarker(t, "failed flush", s, upper, bad)
	wantHold(t, "failed flush", s, []string{badKey}, true)

	// A write that keeps the damaged bytes empties the held set.
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: bad.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	wantMarker(t, "write of the damaged hash", s, upper, bad)
	wantHold(t, "write of the damaged hash", s, nil, false)

	// A write of another block, then a complete flush that uploads every
	// sound block and holds the damaged one again.
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	other := addSyncDirtyBlock(t, ctx, wtx, upper.StoreOps, "other block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: other.GetHash(), Size: int64(len("other block"))}}); err != nil {
		t.Fatal(err)
	}
	failFrom.Store(0)
	var held *heldError
	if err := s.FlushNowUnordered(ctx); !errors.As(err, &held) || held.count != 1 {
		t.Fatalf("complete flush = %v; want exactly the damaged block held", err)
	}
	wantMarker(t, "complete flush", s, upper, bad)
	if n := readPersistedPending(t, s.store).GetCount(); n != 1 {
		t.Fatalf("pending markers = %d; want only the damaged block", n)
	}
	wantHold(t, "complete flush", s, []string{badKey}, false)
}

// TestSyncControllerFailedPushKeepsRecheck holds a block that a write has
// since made sound, with its recheck pending, in a chunk after the first. A
// flush that consumes the recheck and fails its first push stops before that
// chunk. The flush must hand the recheck back so the next flush reads the block
// and uploads it rather than holding it unread.
func TestSyncControllerFailedPushKeepsRecheck(t *testing.T) {
	// Queue the held block, sound on disk, known and due a recheck, behind
	// the chunks. The flushes are unordered so it packs after the first chunk.
	ctx := t.Context()
	s, failFrom, pushes, upper := heldTestController(t)
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	late := addSyncDirtyBlock(t, ctx, wtx, upper.StoreOps, "late block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	s.updateDirtyState(ctx)
	lateKey := late.GetHash().MarshalString()
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.setHold(map[string]struct{}{lateKey: {}}, false)
	})
	s.corruptRecheck.Store(true)

	// The first push fails: the recheck survives and the hold stays.
	failFrom.Store(1)
	var held *heldError
	if err := s.FlushNowUnordered(ctx); err == nil || errors.As(err, &held) {
		t.Fatalf("failing flush = %v; want the push error", err)
	}
	var known map[string]struct{}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { known = s.hold.blocks })
	if !s.corruptRecheck.Load() || !hasKey(known, lateKey) {
		t.Fatalf("after a failed push: recheck %v, held %v; want both kept", s.corruptRecheck.Load(), hasKey(known, lateKey))
	}

	// The next flush reads the block, uploads it and clears the hold.
	failFrom.Store(0)
	pushes.Store(0)
	if err := s.FlushNowUnordered(ctx); err != nil {
		t.Fatalf("flush after the failure = %v; want the block read and uploaded", err)
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { known = s.hold.blocks })
	if len(known) != 0 || pushes.Load() == 0 {
		t.Fatalf("after the flush: %d held, %d pushes; want none held", len(known), pushes.Load())
	}
}

// TestSyncReclaimWalkFailsOnMissingOrCorrupt checks that the reclaim walk
// fails on a root it cannot read, missing everywhere or corrupt in the cache,
// rather than judging the blocks under it dead.
func TestSyncReclaimWalkFailsOnMissingOrCorrupt(t *testing.T) {
	s := newDirtySyncExecuteTestController(t, nil, nil)
	held, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := block.BuildBlockRef([]byte("absent block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: s.upper, bad: held.GetHash()}
	if _, err := s.walkLive(t.Context(), []*block.BlockRef{absent}); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("walk over a missing root = %v; want not found", err)
	}
	if _, err := s.walkLive(t.Context(), []*block.BlockRef{held}); !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("walk over a corrupt root = %v; want corrupt", err)
	}
}
