package block_store_filesegment

import (
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
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

// newIndex returns an Index over an in-memory key/value store.
func newIndex() *Index {
	return NewIndex(store_kvtx_inmem.NewStore(), nil)
}

// newStore returns a Store over idx that reads with xfrm.
func newStore(idx *Index, xfrm block.Transformer) *Store {
	s := NewStore(idx)
	s.SetTransformer(xfrm)
	return s
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

	idx := newIndex()
	linked, err := LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !built.GetChunkIndex().EqualVT(linked.GetChunkIndex()) {
		t.Fatalf("chunk index differs: built %d chunks, linked %d", len(built.GetChunkIndex().GetChunks()), len(linked.GetChunkIndex().GetChunks()))
	}
	t.Logf("%d chunks", len(linked.GetChunkIndex().GetChunks()))
}

// TestReadThroughFile reads a linked file through the Volume read pipeline:
// the Volume holds only the Blob, every chunk of the large file comes from the
// file, and the small file is inlined in its raw Blob.
func TestReadThroughFile(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.bin")
	data := writeRandom(t, path, 5<<20+12345, 2)
	small := filepath.Join(dir, "note.txt")
	smallData := writeRandom(t, small, 3000, 3)

	idx := newIndex()
	fss := newStore(idx, xfrm)
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
		b, err := LinkFile(ctx, idx, tc.path, nil, xfrm, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		if raw := len(tc.want) <= InlineLimit; raw != (b.GetBlobType() == blob.BlobType_BlobType_RAW) {
			t.Fatalf("%s: blob type %v for %d bytes", tc.path, b.GetBlobType(), len(tc.want))
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
// only the hash check stands between the reader and wrong data. The miss drops
// the path's ranges and reports the path for a relink.
func TestChangedFileIsMiss(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	path := filepath.Join(t.TempDir(), "edit.bin")
	data := writeRandom(t, path, 2<<20, 4)
	fi, _ := os.Stat(path)

	idx := newIndex()
	fss := newStore(idx, xfrm)
	var reported []string
	release := fss.WatchHashMisses(func(p string) { reported = append(reported, p) })
	defer release()
	b, err := LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := b.GetChunkIndex().GetChunks()[0]
	last := b.GetChunkIndex().GetChunks()[len(b.GetChunkIndex().GetChunks())-1]

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
	if len(reported) != 1 || reported[0] != path {
		t.Fatalf("hash miss reported %v, want [%s]", reported, path)
	}
	for _, c := range []*blob.Chunk{first, last} {
		if segs, err := idx.Lookup(ctx, c.GetDataRef()); err != nil || len(segs) != 0 {
			t.Fatalf("after the miss the index holds %d ranges of a chunk (err %v), want none", len(segs), err)
		}
	}

	// The relink indexes the changed bytes, which the store then serves.
	b, err = LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	first = b.GetChunkIndex().GetChunks()[0]
	if _, ok, err := fss.GetBlock(ctx, first.GetDataRef()); err != nil || !ok {
		t.Fatalf("relinked range not served: ok=%v err=%v", ok, err)
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

// TestRelinkReplacesPath relinks a changed file and checks that the index
// drops the old ranges in the same write that adds the new ones, that a
// second Index over the same store reads them, and that a dropped path serves
// nothing.
func TestRelinkReplacesPath(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	path := filepath.Join(t.TempDir(), "doc.bin")
	writeRandom(t, path, 3<<20, 7)
	kv := store_kvtx_inmem.NewStore()
	idx := NewIndex(kv, nil)
	before, err := LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite the file and link it again.
	writeRandom(t, path, 3<<20, 8)
	after, err := LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A fresh Index over the store, as after a restart, holds only the new
	// ranges.
	reopened := NewIndex(kv, nil)
	fss := newStore(reopened, xfrm)
	for _, c := range before.GetChunkIndex().GetChunks() {
		segs, err := reopened.Lookup(ctx, c.GetDataRef())
		if err != nil || len(segs) != 0 {
			t.Fatalf("old range kept: %d segments, err %v", len(segs), err)
		}
	}
	for _, c := range after.GetChunkIndex().GetChunks() {
		data, ok, err := fss.GetBlock(ctx, c.GetDataRef())
		if !ok || err != nil {
			t.Fatalf("new range not served: ok=%v err=%v", ok, err)
		}

		// A stat reports the encoded size without reading the file.
		hits := fss.Hits.Load()
		stat, err := fss.StatBlock(ctx, c.GetDataRef())
		if err != nil || stat == nil || stat.Size != int64(len(data)) || fss.Hits.Load() != hits {
			t.Fatalf("stat = %v, %v, want size %d and no read", stat, err, len(data))
		}
	}

	// A copy at a second path shares every ref, and dropping the first path
	// leaves the copy's ranges.
	copyPath := filepath.Join(filepath.Dir(path), "copy.bin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LinkFile(ctx, reopened, copyPath, nil, xfrm, 0, nil); err != nil {
		t.Fatal(err)
	}
	first := after.GetChunkIndex().GetChunks()[0].GetDataRef()
	if segs, err := reopened.Lookup(ctx, first); err != nil || len(segs) != 2 {
		t.Fatalf("shared ref: %d segments, err %v, want 2", len(segs), err)
	}
	if err := reopened.DropPath(ctx, copyPath); err != nil {
		t.Fatal(err)
	}

	// Dropping the path drops every range.
	if err := reopened.DropPath(ctx, path); err != nil {
		t.Fatal(err)
	}
	for _, c := range after.GetChunkIndex().GetChunks() {
		if _, ok, _ := fss.GetBlock(ctx, c.GetDataRef()); ok {
			t.Fatal("dropped range served")
		}
	}
	if _, _, err := fss.PutBlock(ctx, []byte("x"), nil); err != block_store.ErrReadOnly {
		t.Fatalf("PutBlock error = %v, want ErrReadOnly", err)
	}
}

// TestReadScaling links n and then 10n files of equal size and reads every
// chunk once, checking that the cost of a chunk read does not grow with the
// number of linked files: one index key read and one file read per chunk read, and
// the same allocations.
func TestReadScaling(t *testing.T) {
	ctx := context.Background()
	xfrm := spaceTransform(t)
	const fileSize = 200 << 10
	measure := func(n int) (allocs, gets, devReads float64) {
		// Link n files into a counted index.
		dir := t.TempDir()
		kv := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
		idx := NewIndex(kv, nil)
		var refs []*block.BlockRef
		for i := range n {
			path := filepath.Join(dir, strconv.Itoa(i))
			writeRandom(t, path, fileSize, uint64(i)+100) //nolint:gosec // i is small.
			b, err := LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range b.GetChunkIndex().GetChunks() {
				refs = append(refs, c.GetDataRef())
			}
		}

		// Read every chunk once, counting allocations, index key reads and
		// file reads. A warm-up read fills the buffer pool.
		fss := newStore(idx, xfrm)
		if _, ok, err := fss.GetBlock(ctx, refs[0]); !ok || err != nil {
			t.Fatal(ok, err)
		}
		counts0, hits0 := kv.Counts(), fss.Hits.Load()
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		for _, ref := range refs {
			if _, ok, err := fss.GetBlock(ctx, ref); !ok || err != nil {
				t.Fatal(ok, err)
			}
		}
		runtime.ReadMemStats(&m1)
		k := float64(len(refs))
		devReads = float64(fss.Hits.Load()-hits0+fss.HashMisses.Load()) / k
		counts := kv.Counts().Sub(counts0)
		return float64(m1.Mallocs-m0.Mallocs) / k, float64(counts.Reads+counts.Steps) / k, devReads
	}

	a1, g1, d1 := measure(40)
	a10, g10, d10 := measure(400)
	t.Logf("per chunk read: n allocs=%.1f gets=%.2f reads=%.2f; 10n allocs=%.1f gets=%.2f reads=%.2f", a1, g1, d1, a10, g10, d10)
	if g1 != 1 || g10 != 1 || d1 != 1 || d10 != 1 {
		t.Fatal("a chunk read must read one index key and one file")
	}
	if a10 > a1*1.1+1 {
		t.Fatalf("allocations per read grew from %.1f to %.1f", a1, a10)
	}
}

// BenchmarkRegenerate measures one verified chunk read from a warm file.
func BenchmarkRegenerate(b *testing.B) {
	ctx := context.Background()
	xfrm := spaceTransform(b)
	path := filepath.Join(b.TempDir(), "bench.bin")
	writeRandom(b, path, 64<<20, 5)
	idx := newIndex()
	fss := newStore(idx, xfrm)
	bl, err := LinkFile(ctx, idx, path, nil, xfrm, 0, nil)
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
		if _, err := LinkFile(ctx, newIndex(), path, nil, xfrm, 0, nil); err != nil {
			b.Fatal(err)
		}
	}
}
