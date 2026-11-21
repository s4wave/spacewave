package block_store_s3

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
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
	var timed uint64
	for _, n := range extent.ExtentLatency.Counts {
		timed += n
	}
	if timed != 1 {
		t.Fatalf("extent read timed %d reads, want 1", timed)
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

// TestReadCountsExactReads reads one block of a pack through the pack readers
// of a store that has no index, so the read loads the pack's index tail. The
// tail counts apart from the block's payload, and with SetPackExactReads on the
// payload read takes the block's bytes alone, where a window takes its
// neighbors too.
func TestReadCountsExactReads(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	writer := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true})
	t.Cleanup(writer.Close)
	const blockBytes, blockCount = 4 << 10, 8
	var batch []*block.PutBatchEntry
	for i := range blockCount {
		data := bytes.Repeat([]byte{byte('a' + i)}, blockBytes)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
	}
	if _, err := writer.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}

	defer SetPackExactReads(false)
	for _, exact := range []bool{false, true} {
		SetPackExactReads(exact)
		reader := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true})
		if _, found, err := reader.GetBlock(ctx, batch[blockCount/2].Ref); err != nil || !found {
			reader.Close()
			t.Fatalf("exact=%v: read = %v, %v", exact, found, err)
		}
		got := reader.ReadCounts()
		reader.Close()
		if got.TailFetches == 0 || got.TailBytes == 0 || got.WindowFetches != 1 {
			t.Fatalf("exact=%v: %+v, want one payload read after a tail read", exact, got)
		}
		if exact && got.WindowBytes > blockBytes+64 {
			t.Fatalf("exact read downloaded %d payload bytes for a %d byte block", got.WindowBytes, blockBytes)
		}
		if !exact && got.WindowBytes < 2*blockBytes {
			t.Fatalf("window read downloaded %d payload bytes, want its neighbors", got.WindowBytes)
		}
	}
}
