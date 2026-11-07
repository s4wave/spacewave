package block_store_s3

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// windowSink records the blocks a window fill writes.
type windowSink struct {
	block.StoreOps
	mtx    sync.Mutex
	blocks map[string][]byte
}

func (w *windowSink) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, e := range entries {
		w.blocks[e.Ref.MarshalString()] = e.Data
	}
	return make([]bool, len(entries)), nil
}

func (w *windowSink) count() int {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return len(w.blocks)
}

// TestPackStoreWindowFill checks that one bucket read of a pack fills every
// complete block of the window it fetched, not only the block asked for: the
// whole 400 KiB pack fits the 1 MiB window.
func TestPackStoreWindowFill(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)

	// Write one pack of 400 blocks of 1 KiB, about 400 KiB.
	var batch []*block.PutBatchEntry
	for i := range 400 {
		data := append([]byte(fmt.Sprintf("block %d ", i)), bytes.Repeat([]byte{byte(i)}, 1024)...)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data})
	}
	if _, err := newTestPackStore(t, client).PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}

	// A fresh store reads one block and fills all 400.
	reader := newTestPackStore(t, client)
	sink := &windowSink{blocks: make(map[string][]byte)}
	reader.SetWindowFill(ctx, sink)
	data, found, err := reader.GetBlock(ctx, batch[200].Ref)
	if err != nil || !found || !bytes.Equal(data, batch[200].Data) {
		t.Fatalf("read = %v, %v", found, err)
	}
	waitFor(t, "the window fills every block", func() bool { return sink.count() == len(batch) })
	for _, e := range batch {
		sink.mtx.Lock()
		got := sink.blocks[e.Ref.MarshalString()]
		sink.mtx.Unlock()
		if !bytes.Equal(got, e.Data) {
			t.Fatalf("filled block %s differs", e.Ref.MarshalString())
		}
	}
}
