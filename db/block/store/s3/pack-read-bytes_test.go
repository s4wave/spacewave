package block_store_s3

import (
	"testing"

	"github.com/s4wave/spacewave/db/kvtx/hashmap"
)

// TestReadCountsDownloadBytes counts the bytes a store downloads by read mode:
// an extent read counts its one range, and a read through the pack readers
// counts their windows.
func TestReadCountsDownloadBytes(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	s := newIndexedStore(t, client, kv)
	refs := adoptBlocks(ctx, t, s, "w", 3)
	if adopted := s.ReadCounts(); adopted.ExtentBytes != 0 || adopted.WindowBytes != 0 || adopted.WholeBytes != 0 {
		t.Fatalf("after the adopt: %+v, want no block bytes", adopted)
	}

	// An extent read downloads the block's extent alone.
	SetIndexExtentReads(true)
	if _, found, err := s.GetBlock(ctx, refs[0]); err != nil || !found {
		t.Fatalf("extent read = %v, %v", found, err)
	}
	extent := s.ReadCounts()
	if extent.Bucket != 1 || extent.ExtentBytes == 0 || extent.WindowBytes != 0 {
		t.Fatalf("after the extent read: %+v", extent)
	}
	if want := uint64(len("w block 0")); extent.ExtentBytes < want {
		t.Fatalf("extent read counted %d bytes, want at least %d", extent.ExtentBytes, want)
	}

	// With extent reads off, the pack readers download windows.
	SetIndexExtentReads(false)
	defer SetIndexExtentReads(true)
	if _, found, err := s.GetBlock(ctx, refs[1]); err != nil || !found {
		t.Fatalf("window read = %v, %v", found, err)
	}
	window := s.ReadCounts()
	if window.WindowBytes == 0 || window.ExtentBytes != extent.ExtentBytes {
		t.Fatalf("after the window read: %+v", window)
	}
	t.Logf("%+v", window)
}
