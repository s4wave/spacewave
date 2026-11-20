package block_store_filesegment

import (
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

func spaceTransform(t testing.TB) block.Transformer {
	enc, err := transform_blockenc.NewBlockEnc(&transform_blockenc.Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      bytes.Repeat([]byte{7}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	return block_transform.NewTransformerWithSteps([]block_transform.Step{enc})
}

func writeRandom(t testing.TB, path string, n int, seed uint64) []byte {
	data := make([]byte, n)
	r := rand.New(rand.NewPCG(seed, seed))
	for i := range data {
		data[i] = byte(r.Uint32())
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return data
}

// TestAddressMatchesBuildBlob checks that addressing a file yields the chunk
// index BuildBlob stores for the same bytes, so the Blob a linked folder
// writes is the Blob any other writer would.
func TestAddressMatchesBuildBlob(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	path := filepath.Join(t.TempDir(), "big.bin")
	data := writeRandom(t, path, 12<<20, 1)

	store := block_mock.NewMockStore(0)
	btx, bcs := block.NewTransaction(store, xfrm, nil, nil)
	built, err := blob.BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), bcs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := btx.Write(ctx, true); err != nil {
		t.Fatal(err)
	}

	idx := NewIndex()
	linked, err := LinkFile(ctx, idx, path, nil, xfrm, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !built.GetChunkIndex().EqualVT(linked.GetChunkIndex()) {
		t.Fatalf("chunk index differs: built %d chunks, linked %d", len(built.GetChunkIndex().GetChunks()), len(linked.GetChunkIndex().GetChunks()))
	}
	t.Logf("%d chunks, %d indexed refs", len(linked.GetChunkIndex().GetChunks()), idx.Len())
}

// TestReadThroughFile reads a linked file through the Volume read pipeline:
// the Volume holds only the Blob, and every chunk comes from the file.
func TestReadThroughFile(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.bin")
	data := writeRandom(t, path, 5<<20+12345, 2)
	small := filepath.Join(dir, "note.txt")
	smallData := writeRandom(t, small, 3000, 3)

	idx := NewIndex()
	fss := NewStore(idx, xfrm)
	volume := block_mock.NewMockStore(0)
	pipeline := block_store.NewStoreReadThrough(
		func() block.StoreOps { return volume },
		func() block.StoreOps { return fss },
		nil,
	)

	for _, tc := range []struct {
		path string
		want []byte
	}{{path, data}, {small, smallData}} {
		b, err := LinkFile(ctx, idx, tc.path, nil, xfrm, 0)
		if err != nil {
			t.Fatal(err)
		}

		// Write only the Blob to the Volume.
		wtx, wcs := block.NewTransaction(volume, xfrm, nil, nil)
		wcs.SetBlock(b, true)
		root, _, err := wtx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

		// Read it back through the pipeline.
		_, rcs := block.NewTransaction(pipeline, xfrm, root, nil)
		rdr, err := blob.NewReader(ctx, rcs)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rdr)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, tc.want) {
			t.Fatalf("%s: read %d bytes, want %d", tc.path, len(got), len(tc.want))
		}
	}
	t.Logf("hits=%d", fss.Hits.Load())
}

// TestChangedFileIsMiss tries to make the store serve stale bytes: it changes
// a byte, keeps the size and restores the mtime, so the stat check passes and
// only the hash check stands between the reader and wrong data.
func TestChangedFileIsMiss(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	path := filepath.Join(t.TempDir(), "edit.bin")
	data := writeRandom(t, path, 2<<20, 4)
	fi, _ := os.Stat(path)

	idx := NewIndex()
	fss := NewStore(idx, xfrm)
	b, err := LinkFile(ctx, idx, path, nil, xfrm, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := b.GetChunkIndex().GetChunks()[0]

	data[10] ^= 0xff
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Now(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}

	got, ok, err := fss.GetBlock(ctx, first.GetDataRef())
	if err != nil || ok || got != nil {
		t.Fatalf("changed range served: ok=%v err=%v", ok, err)
	}
	if fss.HashMisses.Load() != 1 {
		t.Fatalf("hash misses = %d, want 1", fss.HashMisses.Load())
	}

	// A size change is a stat miss with no read.
	if err := os.WriteFile(path, data[:len(data)-1], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := fss.GetBlock(ctx, first.GetDataRef()); ok {
		t.Fatal("truncated file served")
	}
	if fss.StatMisses.Load() != 1 {
		t.Fatalf("stat misses = %d, want 1", fss.StatMisses.Load())
	}
}

// BenchmarkRegenerate measures one verified chunk read from a warm file.
func BenchmarkRegenerate(b *testing.B) {
	ctx := context.Background()
	xfrm := spaceTransform(b)
	path := filepath.Join(b.TempDir(), "bench.bin")
	writeRandom(b, path, 64<<20, 5)
	idx := NewIndex()
	fss := NewStore(idx, xfrm)
	bl, err := LinkFile(ctx, idx, path, nil, xfrm, 0)
	if err != nil {
		b.Fatal(err)
	}
	chunks := bl.GetChunkIndex().GetChunks()
	var bytesRead int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := chunks[i%len(chunks)]
		if _, ok, err := fss.GetBlock(ctx, c.GetDataRef()); !ok || err != nil {
			b.Fatal(ok, err)
		}
		bytesRead += int64(c.GetSize())
	}
	b.SetBytes(bytesRead / int64(b.N))
}

// BenchmarkLink measures addressing a file: chunk, encode and hash.
func BenchmarkLink(b *testing.B) {
	ctx := context.Background()
	xfrm := spaceTransform(b)
	path := filepath.Join(b.TempDir(), "bench.bin")
	writeRandom(b, path, 64<<20, 6)
	b.SetBytes(64 << 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := LinkFile(ctx, NewIndex(), path, nil, xfrm, 0); err != nil {
			b.Fatal(err)
		}
	}
}
