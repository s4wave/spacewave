package provider_local

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackaheadReserveRefusesPublication publishes through a placed store
// whose diverting log keeps a disk reserve. Below the reserve a publication
// fails with ErrReserve and moves no head, so nothing is acknowledged, while
// the blocks published before still read; once the space returns the next
// publication lands on the head the first one left.
func TestPackaheadReserveRefusesPublication(t *testing.T) {
	// Open the placed store with a log whose free disk the test sets.
	ctx := t.Context()
	local, vol := newOwnedVolume(t)
	markers, release, err := vol.AccessObjectStore(ctx, "test/markers", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	var free atomic.Uint64
	free.Store(1 << 30)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{
		Linger:       10 * time.Millisecond,
		ReserveBytes: 64 << 20,
		FreeBytes:    func(string) (uint64, error) { return free.Load(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	packed := block_store_writeback.NewPackedStore(log, local, true)
	wb, err := block_store_writeback.NewStoreWithPacks(ctx, packed, vol, markers, "test/markers", true)
	if err != nil {
		t.Fatal(err)
	}
	tracker, _ := vol.GetRefGraph().(block_gc.BlockTracker)
	p := &placementState{wb: wb, local: packed, packed: packed, tracker: tracker}
	p.backend.Store(&account_settings.StorageBackend{Id: "packs"})
	b := &BlockStore{store: block_store.NewStore("local", wb), decodedBlocks: block.NewDecodedBlockCache(), placement: p}
	t.Cleanup(b.decodedBlocks.Close)

	// publish moves the head from want to next with one new block.
	publish := func(data, want, next string) error {
		ref, _ := block.BuildBlockRef([]byte(data), nil)
		return b.PublishAtomic(ctx, &block.AtomicPublication{
			Entries:  []*block.PutBatchEntry{{Ref: ref, Data: []byte(data), RefsKnown: true}},
			BucketID: "bucket",
			TrackGC:  true,
			Heads: []*block.AtomicHeadUpdate{{ObjectStoreID: "head", Key: []byte("head"), Replace: func(_ context.Context, current []byte, found bool) ([]byte, error) {
				if string(current) != want || found != (want != "") {
					return nil, errors.New("head is " + string(current) + ", want " + want)
				}
				return []byte(next), nil
			}}},
		})
	}

	// Above the reserve the first publication lands.
	if err := publish("first block", "", "first"); err != nil {
		t.Fatal(err)
	}

	// Below the reserve the second fails cleanly and stores nothing.
	free.Store(32 << 20)
	if err := publish("second block", "first", "second"); !errors.Is(err, packahead.ErrReserve) {
		t.Fatalf("publication below the reserve returned %v, want ErrReserve", err)
	}
	secondRef, _ := block.BuildBlockRef([]byte("second block"), nil)
	if _, found, _ := b.GetBlock(ctx, secondRef); found {
		t.Fatal("the refused block reads")
	}

	// The first publication's block still reads below the reserve.
	firstRef, _ := block.BuildBlockRef([]byte("first block"), nil)
	got, found, err := b.GetBlock(ctx, firstRef)
	if err != nil || !found || !bytes.Equal(got, []byte("first block")) {
		t.Fatalf("read below the reserve: %v %v", found, err)
	}

	// Once the space returns the publication lands on the first head.
	free.Store(1 << 30)
	if err := publish("second block", "first", "second"); err != nil {
		t.Fatalf("publication after the space returned: %v", err)
	}
	if st := packed.GetStats(); st.Diverted != 2 {
		t.Fatalf("diverted %d blocks, want the two published", st.Diverted)
	}
}
