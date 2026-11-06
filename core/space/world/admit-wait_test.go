//go:build !goscript

package space_world

import (
	"encoding/binary"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// TestAdmitWaitsOnlyForTheRootSwap checks, at two sizes of a checkpoint's
// kept World, that a local write's payload hold landing while the driver
// stores that World under the checkpoint root waits only for the root swap,
// not for the store: its longest wait stays a small part of the store's time
// and does not grow with the World.
func TestAdmitWaitsOnlyForTheRootSwap(t *testing.T) {
	type result struct{ wait, store time.Duration }
	results := make(map[int]result)
	for _, mib := range []int{4, 32} {
		var r result
		for range 3 {
			wait, store := measureAdmitWait(t, mib)
			r.wait, r.store = max(r.wait, wait), max(r.store, store)
		}
		t.Logf("checkpoint of %d MiB: store %v, longest payload hold wait %v", mib, r.store, r.wait)
		results[mib] = r
	}
	large := results[32]
	if large.wait > large.store/4 {
		t.Fatalf("a payload hold waited %v behind a %v checkpoint store", large.wait, large.store)
	}
}

// measureAdmitWait stores a kept World of mib MiB under the checkpoint root,
// as Root does, and returns how long a payload hold, as Admit makes it, waited
// once it landed during the store, and how long the store took.
func measureAdmitWait(t *testing.T, mib int) (wait, store time.Duration) {
	// Keep a World of mib MiB of 4 KiB pages, two levels deep, and a payload.
	ctx := t.Context()
	a, _ := newVolumeAdapter(t, (mib+8)<<20)
	put := func(data []byte, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, Refs: children, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	var nodes []*block.BlockRef
	page := make([]byte, 4<<10)
	for n := range mib << 8 / 256 {
		leaves := make([]*block.BlockRef, 256)
		for i := range leaves {
			binary.LittleEndian.PutUint64(page, uint64(n<<8|i)) //nolint:gosec // small and positive.
			leaves[i] = put(slices.Clone(page))
		}
		nodes = append(nodes, put([]byte("node "+strconv.Itoa(n)), leaves...))
	}
	world := put([]byte("checkpoint World"), nodes...)
	payload := put([]byte("payload"))
	a.replay = &spaceReplay{a: a, base: &InnerState{}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: world}}}}

	// Start the checkpoint's store, then hold the payload during it.
	stored := make(chan error, 1)
	start := time.Now()
	go func() {
		_, store, err := a.Root(ctx, 1)
		if err == nil {
			err = store(ctx)
		}
		stored <- err
	}()
	time.Sleep(2 * time.Millisecond)
	adm := &spaceAdmission{payloads: []*block.BlockRef{payload}, done: make(chan struct{})}
	held := time.Now()
	if err := a.holdAdmitted(ctx, adm); err != nil {
		t.Fatal(err)
	}
	wait = time.Since(held)
	if err := <-stored; err != nil {
		t.Fatal(err)
	}
	return wait, time.Since(start)
}
