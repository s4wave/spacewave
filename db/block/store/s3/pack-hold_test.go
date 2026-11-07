//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// openSweepStore opens a store that keeps its packs on the bucket, with its
// index in kv. The caller closes it.
func openSweepStore(client ObjectStore, kv kvtx.Store) *PackStore {
	return NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
}

// heldState reads the state the index holds pack id in.
func heldStateOf(t *testing.T, s *PackStore, id string) packState {
	t.Helper()
	state, ok := s.index.heldState(id)
	if !ok {
		t.Fatalf("the index holds no pack %s", id)
	}
	return state
}

// locatedAny reports whether the index places any of refs.
func locatedAny(t *testing.T, s *PackStore, refs []*block.BlockRef) bool {
	t.Helper()
	found, err := s.LocateBlocks(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(found, true)
}

// TestQuarantineReadLifts quarantines a pack, finds it skipped by the extent
// read, the locate and the existence check, across a restart, and finds a
// read of one of its blocks served by the fallback, which lifts the pack and
// records the read.
func TestQuarantineReadLifts(t *testing.T) {
	eachIndexVolume(t, func(t *testing.T, kind string) {
		ctx := t.Context()
		_, client := newFakeBucket(t, nil)
		kv := newIndexKV(t, kind)
		s := openSweepStore(client, kv)
		live := adoptBlocks(ctx, t, s, "live", 2)
		dead := adoptBlocks(ctx, t, s, "dead", 2)
		deadPack := packOf(t, s, dead[0])

		// Quarantine the pack twice: the second call changes nothing.
		for range 2 {
			if err := s.Quarantine(ctx, []string{deadPack}); err != nil {
				t.Fatal(err)
			}
		}
		if got := heldStateOf(t, s, deadPack); got != packQuarantined {
			t.Fatalf("state %v, want quarantined", got)
		}
		s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
			if s.entries[deadPack] != nil || s.heldEntries[deadPack] == nil {
				t.Error("the quarantined entry did not move to the held manifest")
			}
		})

		// Locate, the existence checks and the extent read skip it.
		if locatedAny(t, s, dead) {
			t.Fatal("located a block only a quarantined pack holds")
		}
		if found, err := s.GetBlockExists(ctx, dead[0]); err != nil || found {
			t.Fatalf("exists = %v, %v; want false", found, err)
		}
		if found, err := s.GetBlockExistsBatch(ctx, dead); err != nil || slices.Contains(found, true) {
			t.Fatalf("exists batch = %v, %v; want none", found, err)
		}
		if s.readExtent(ctx, dead[0]) != nil {
			t.Fatal("the extent read served a quarantined pack")
		}
		if data, found, err := s.GetBlock(ctx, live[0]); err != nil || !found || string(data) != "live block 0" {
			t.Fatalf("live read = %q, %v, %v", data, found, err)
		}

		// The state survives a restart.
		s.Close()
		s = openSweepStore(client, kv)
		defer s.Close()
		if locatedAny(t, s, dead) {
			t.Fatal("located a quarantined block after a restart")
		}
		if got := heldStateOf(t, s, deadPack); got != packQuarantined {
			t.Fatalf("state after restart %v, want quarantined", got)
		}

		// A read falls back, lifts the pack and records the read.
		if data, found, err := s.GetBlock(ctx, dead[0]); err != nil || !found || string(data) != "dead block 0" {
			t.Fatalf("fallback read = %q, %v, %v", data, found, err)
		}
		if got := heldStateOf(t, s, deadPack); got != packRegistered {
			t.Fatalf("state after the read %v, want registered", got)
		}
		hold, ok, err := s.index.readHold(ctx, deadPack)
		if err != nil || !ok || hold.state != packRegistered || hold.event == 0 || !bytes.Equal(hold.key, packfile.BlockKey(dead[0].GetHash())) {
			t.Fatalf("lift record = %+v, %v, %v", hold, ok, err)
		}
		if !locatedAny(t, s, dead[1:]) {
			t.Fatal("the lifted pack places none of its blocks")
		}
		if found, err := s.GetBlockExists(ctx, dead[1]); err != nil || !found {
			t.Fatalf("exists after the lift = %v, %v", found, err)
		}
	})
}

// TestQuarantineStatLifts finds a stat of a block only a quarantined pack
// holds answered by the fallback, which lifts the pack.
func TestQuarantineStatLifts(t *testing.T) {
	eachIndexVolume(t, func(t *testing.T, kind string) {
		ctx := t.Context()
		_, client := newFakeBucket(t, nil)
		s := openSweepStore(client, newIndexKV(t, kind))
		defer s.Close()
		dead := adoptBlocks(ctx, t, s, "dead", 2)
		deadPack := packOf(t, s, dead[0])
		if err := s.Quarantine(ctx, []string{deadPack}); err != nil {
			t.Fatal(err)
		}
		if stat, err := s.StatBlock(ctx, dead[1]); err != nil || stat == nil {
			t.Fatalf("stat = %v, %v", stat, err)
		}
		if got := heldStateOf(t, s, deadPack); got != packRegistered {
			t.Fatalf("state after the stat %v, want registered", got)
		}
	})
}

// TestQuarantineMoves checks the moves the states refuse, the command's lift,
// the miss a trashing pack records, the held packs Packs reports, and that a
// remove forgets the pack's row.
func TestQuarantineMoves(t *testing.T) {
	eachIndexVolume(t, func(t *testing.T, kind string) {
		ctx := t.Context()
		_, client := newFakeBucket(t, nil)
		kv := newIndexKV(t, kind)
		s := openSweepStore(client, kv)
		defer s.Close()
		a := adoptBlocks(ctx, t, s, "a", 2)
		b := adoptBlocks(ctx, t, s, "b", 2)
		packA, packB := packOf(t, s, a[0]), packOf(t, s, b[0])

		// A pack the index lacks, and a store that may rewrite its packs,
		// are refused.
		if err := s.Quarantine(ctx, []string{packA, "nope"}); err == nil {
			t.Fatal("quarantined a pack the index lacks")
		}
		if got := heldStateOf(t, s, packA); got != packRegistered {
			t.Fatalf("a refused call moved pack a to %v", got)
		}
		rewriting := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{Index: newIndexKV(t, "memory"), SoleWriter: true})
		defer rewriting.Close()
		if err := rewriting.Quarantine(ctx, []string{packA}); err == nil {
			t.Fatal("quarantined on a store that rewrites its packs")
		}

		// The command's lift records no block.
		if err := s.Quarantine(ctx, []string{packA, packB}); err != nil {
			t.Fatal(err)
		}
		packs, err := s.Packs(ctx)
		if err != nil || len(packs) != 2 {
			t.Fatalf("Packs = %d packs, %v; want both held packs", len(packs), err)
		}
		if err := s.Lift(ctx, []string{packA, packA}); err != nil {
			t.Fatal(err)
		}
		hold, ok, err := s.index.readHold(ctx, packA)
		if err != nil || !ok || hold.state != packRegistered || hold.event == 0 || len(hold.key) != 0 {
			t.Fatalf("command lift record = %+v, %v, %v", hold, ok, err)
		}
		if !locatedAny(t, s, a) {
			t.Fatal("the lifted pack places none of its blocks")
		}

		// A trashing pack refuses quarantine and lift; a read still serves it
		// and records the miss, keeping it trashing.
		release, err := s.holdMtx.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.index.writeHolds(ctx, []string{packB}, []packHold{{state: packTrashing, since: 1}}); err != nil {
			t.Fatal(err)
		}
		s.setHolds([]string{packB}, packTrashing)
		release()
		if err := s.Quarantine(ctx, []string{packB}); err == nil {
			t.Fatal("quarantined a trashing pack")
		}
		if err := s.Lift(ctx, []string{packB}); err == nil {
			t.Fatal("lifted a trashing pack")
		}
		if data, found, err := s.GetBlock(ctx, b[1]); err != nil || !found || string(data) != "b block 1" {
			t.Fatalf("trashing read = %q, %v, %v", data, found, err)
		}
		hold, ok, err = s.index.readHold(ctx, packB)
		if err != nil || !ok || hold.state != packTrashing || hold.since != 1 || hold.event == 0 || !bytes.Equal(hold.key, packfile.BlockKey(b[1].GetHash())) {
			t.Fatalf("miss record = %+v, %v, %v", hold, ok, err)
		}

		// A remove forgets the row and the state.
		s.updateEntries(nil, []string{packB})
		if n := rowCount(t, kv, indexHoldPrefix); n != 1 {
			t.Fatalf("%d q/ rows after the remove, want pack a's lift", n)
		}
		if s.index.heldCount.Load() != 0 {
			t.Fatal("the removed pack is still held")
		}
	})
}

// TestQuarantineConcurrentReads reads every block of a pack while it is
// quarantined and lifted over and over: each read finds its block, from the
// main manifest or the fallback.
func TestQuarantineConcurrentReads(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	s := openSweepStore(client, newIndexKV(t, "memory"))
	defer s.Close()
	refs := adoptBlocks(ctx, t, s, "hot", 8)
	id := packOf(t, s, refs[0])

	// Read the blocks until the moves end.
	var done atomic.Bool
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := range 8 {
		wg.Go(func() {
			for n := 0; !done.Load(); n++ {
				ref := refs[(i+n)%len(refs)]
				data, found, err := s.GetBlock(ctx, ref)
				if err != nil || !found {
					errs <- fmt.Errorf("read %d: %v, %v", n, found, err)
					return
				}
				if want := fmt.Sprintf("hot block %d", (i+n)%len(refs)); string(data) != want {
					errs <- fmt.Errorf("read %q, want %q", data, want)
					return
				}
			}
		})
	}

	// Quarantine and lift the pack.
	for range 200 {
		if err := s.Quarantine(ctx, []string{id}); err != nil {
			t.Fatal(err)
		}
		if err := s.Lift(ctx, []string{id}); err != nil {
			t.Fatal(err)
		}
	}
	done.Store(true)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
