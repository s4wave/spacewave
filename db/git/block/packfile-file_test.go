package git_block

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestPackfileFileSeeksWithoutMaterializingArchive protects bounded reads and
// independent positional reads on a persisted chunked blob.
func TestPackfileFileSeeksWithoutMaterializingArchive(t *testing.T) {
	const count, size = 16, 64 << 10
	file, store := newCountedPackfileFile(t, count, size)

	// Read the last bytes positionally, then verify sequential reads still start at zero.
	data := make([]byte, 3)
	n, err := file.ReadAt(data, count*size-2)
	if n != 2 || err != io.EOF || !bytes.Equal(data[:n], []byte{count, count}) {
		t.Fatalf("tail: %d %v %v", n, data, err)
	}
	if _, err := io.ReadFull(file, data); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte{1, 1, 1}) {
		t.Fatalf("ReadAt changed sequential position: %v", data)
	}
	if fetched := store.bytes.Load(); fetched >= 3*size {
		t.Fatalf("small reads fetched %d bytes from a %d-byte archive", fetched, count*size)
	}

	// Closing releases the reader and rejects all later reads.
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Read(data); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}

// TestPackfileFileRevisitsRetainedChunks protects object lookups, which reread
// the pack header before each object, from fetching the same chunks again.
func TestPackfileFileRevisitsRetainedChunks(t *testing.T) {
	const count, size = 16, 64 << 10
	file, store := newCountedPackfileFile(t, count, size)

	// Alternate between the header chunk and two object chunks.
	data := make([]byte, 12)
	for range 10 {
		for _, idx := range []int{0, 5, 0, 11} {
			if _, err := file.Seek(int64(idx*size), io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if _, err := io.ReadFull(file, data); err != nil {
				t.Fatal(err)
			}
			if data[0] != byte(idx+1) {
				t.Fatalf("chunk %d read %v", idx, data)
			}
		}
	}
	if fetched := store.bytes.Load(); fetched >= 4*size {
		t.Fatalf("revisiting three chunks fetched %d bytes, want under %d", fetched, 4*size)
	}
}

// newCountedPackfileFile persists a multi-chunk archive and opens it through
// storage that counts fetched bytes.
func newCountedPackfileFile(t *testing.T, count, size int) (*PackfileFile, *packfileReadStore) {
	t.Helper()
	ctx := t.Context()
	base := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(base, nil, nil, nil)
	root := &blob.Blob{
		BlobType:   blob.BlobType_BlobType_CHUNKED,
		TotalSize:  uint64(count * size),
		ChunkIndex: &blob.ChunkIndex{},
	}
	cursor.SetBlock(root, true)
	chunks := root.ChunkIndex.GetChunkSet(cursor.FollowSubBlock(4))
	for idx := range count {
		data := bytes.Repeat([]byte{byte(idx + 1)}, size)
		root.ChunkIndex.AppendChunk(chunks, idx, uint64(size), uint64(idx*size), data)
	}
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	store := &packfileReadStore{StoreOps: base}
	_, cursor = block.NewTransaction(store, nil, ref, nil)
	file, err := NewPackfileFile(ctx, "test.pack", cursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file, store
}

// packfileReadStore counts the bytes fetched from its store.
type packfileReadStore struct {
	block.StoreOps
	bytes atomic.Int64
}

func (s *packfileReadStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	s.bytes.Add(int64(len(data)))
	return data, found, err
}
