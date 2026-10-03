package store

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestPackReaderBlockReadAcrossEviction reads fragmented blocks while a shared
// budget cannot retain both halves, with one reader and concurrent readers.
func TestPackReaderBlockReadAcrossEviction(t *testing.T) {
	for _, readers := range []int{1, 8} {
		t.Run(strconv.Itoa(readers), func(t *testing.T) {
			// Share a budget smaller than one block across independent packs.
			budget := newResidentBudget(1)
			for i := range readers {
				t.Run(strconv.Itoa(i), func(t *testing.T) {
					// Load the real pack index from cache without warming payloads.
					item := testPackItem(t, "a block split across resident spans")
					data, _ := packItems(t, []packItem{item})
					cache := newMemIndexCache()
					if err := cache.Set(t.Context(), "pack", mustReadIndexTail(t, data)); err != nil {
						t.Fatal(err)
					}

					// Create a pack reader with a one-byte shared budget and a cached index.
					transport := &bytesTransport{data: data}
					reader := NewPackReader("pack", int64(len(data)), transport)
					t.Cleanup(reader.Close)
					reader.setBudget(budget)
					reader.setTransportWindows(1, 1, 1)
					reader.SetExpectedBlockCount(1)
					reader.SetIndexCache(cache)
					if err := reader.ensureIndexLoaded(t.Context()); err != nil {
						t.Fatal(err)
					}

					// Seed a middle byte so gap planning must split the block fetch.
					key := packfile.BlockKey(item.h)
					var off, end int64
					reader.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
						entry, _ := reader.findEntryByKeyLocked(key)
						off, end = entryExtent(entry)
					})
					if _, err := reader.ReaderAt(t.Context()).ReadAt(make([]byte, 1), (off+end)/2); err != nil {
						t.Fatal(err)
					}
					t.Parallel()

					// The read must retain every fetched byte until verification.
					stored, err := reader.getBlock(t.Context(), key, block.NewBlockRef(item.h))
					if err != nil {
						t.Fatalf("getBlock with a one-byte shared budget: %v", err)
					}
					if !bytes.Equal(stored.GetData(), item.data) {
						t.Fatalf("block data = %q, want %q", stored.GetData(), item.data)
					}
				})
			}
		})
	}
}

// TestPackReaderIndexTailAcrossEviction keeps an index suffix readable while
// gap fetches evict its earlier spans from a one-byte cache.
func TestPackReaderIndexTailAcrossEviction(t *testing.T) {
	// Seed the final trailer byte to split the cold index-tail request.
	item := testPackItem(t, "a block whose index crosses resident spans")
	data, _ := packItems(t, []packItem{item})
	reader := NewPackReader("pack", int64(len(data)), &bytesTransport{data: data})
	t.Cleanup(reader.Close)
	reader.budget.limit.Store(1)
	reader.setTransportWindows(1, 1, 1)
	reader.SetExpectedBlockCount(1)
	if _, err := reader.ReaderAt(t.Context()).ReadAt(make([]byte, 1), int64(len(data)-1)); err != nil {
		t.Fatal(err)
	}

	// Index loading and the following block read must survive their own eviction.
	stored, err := reader.getBlock(t.Context(), packfile.BlockKey(item.h), block.NewBlockRef(item.h))
	if err != nil {
		t.Fatalf("getBlock with a fragmented cold index: %v", err)
	}
	if !bytes.Equal(stored.GetData(), item.data) {
		t.Fatalf("block data = %q, want %q", stored.GetData(), item.data)
	}
}
