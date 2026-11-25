package provider_local

import (
	"bytes"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackaheadReserveRefusesPublication writes through a placed store whose
// diverting log keeps a disk reserve. Below the reserve a write fails with
// ErrReserve and stores nothing, so nothing is acknowledged, while the blocks
// written before still read; once the space returns the next write lands.
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
	p := &placementState{wb: wb, local: packed, packed: packed}
	p.backend.Store(&account_settings.StorageBackend{Id: "packs"})
	b := &BlockStore{store: block_store.NewStore("local", wb), decodedBlocks: block.NewDecodedBlockCache(), placement: p}
	t.Cleanup(b.decodedBlocks.Close)

	// write stores one new block.
	write := func(data string) error {
		ref, _ := block.BuildBlockRef([]byte(data), nil)
		_, err := b.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte(data), RefsKnown: true}})
		return err
	}

	// Above the reserve the first write lands.
	if err := write("first block"); err != nil {
		t.Fatal(err)
	}

	// Below the reserve the second fails cleanly and stores nothing.
	free.Store(32 << 20)
	if err := write("second block"); !errors.Is(err, packahead.ErrReserve) {
		t.Fatalf("write below the reserve returned %v, want ErrReserve", err)
	}
	secondRef, _ := block.BuildBlockRef([]byte("second block"), nil)
	if _, found, _ := b.GetBlock(ctx, secondRef); found {
		t.Fatal("the refused block reads")
	}

	// The first write's block still reads below the reserve.
	firstRef, _ := block.BuildBlockRef([]byte("first block"), nil)
	got, found, err := b.GetBlock(ctx, firstRef)
	if err != nil || !found || !bytes.Equal(got, []byte("first block")) {
		t.Fatalf("read below the reserve: %v %v", found, err)
	}

	// Once the space returns the write lands.
	free.Store(1 << 30)
	if err := write("second block"); err != nil {
		t.Fatalf("write after the space returned: %v", err)
	}
	if st := packed.GetStats(); st.Diverted != 2 {
		t.Fatalf("diverted %d blocks, want the two written", st.Diverted)
	}
}
