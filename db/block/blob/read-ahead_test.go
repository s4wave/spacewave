package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

func TestBlobReaderReadAheadBoundsAndClose(t *testing.T) {
	for _, tc := range []struct {
		name        string
		chunkSize   int
		count       int
		wantFetches int
	}{
		{name: "concurrency", chunkSize: 64 << 10, count: 12, wantFetches: 8},
		{name: "bytes", chunkSize: 2 << 20, count: 6, wantFetches: 2},
		{name: "oversized-demand", chunkSize: 5 << 20, count: 2, wantFetches: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			reader, store := newReadAheadFixture(t, ctx, tc.count, tc.chunkSize)
			defer reader.Close()

			buf := make([]byte, 32<<10)
			readDone := make(chan error, 1)
			go func() {
				_, err := io.ReadFull(reader, buf)
				readDone <- err
			}()
			seen := make(map[int]bool)
			for range tc.wantFetches {
				seen[waitChunkFetch(t, ctx, store)] = true
			}
			for idx := range tc.wantFetches {
				if !seen[idx] {
					t.Fatalf("window omitted chunk %d: %v", idx, seen)
				}
			}
			close(store.gates[0])
			if err := <-readDone; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf, bytes.Repeat([]byte{1}, len(buf))) {
				t.Fatal("first chunk bytes changed")
			}

			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			if got := store.active.Load(); got != 0 {
				t.Fatalf("Close returned with %d active reads", got)
			}
			if got := store.calls.Load(); got != int32(tc.wantFetches) {
				t.Fatalf("fetched %d chunks, want bounded window of %d", got, tc.wantFetches)
			}
		})
	}
}

func TestBlobReaderSmallReadDoesNotReadAhead(t *testing.T) {
	reader, store := newReadAheadFixture(t, t.Context(), 3, 64<<10)
	defer reader.Close()
	close(store.gates[0])
	if _, err := io.ReadFull(reader, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if got := store.calls.Load(); got != 1 {
		t.Fatalf("small read fetched %d chunks, want 1", got)
	}
}

func TestBlobReaderReadAheadPreservesErrorOrder(t *testing.T) {
	reader, store := newReadAheadFixture(t, t.Context(), 3, 64<<10)
	defer reader.Close()
	wantErr := errors.New("unavailable second chunk")
	store.failIdx, store.failErr = 1, wantErr
	for _, gate := range store.gates {
		close(gate)
	}

	buf := make([]byte, 3*64<<10)
	n, err := io.ReadFull(reader, buf)
	if n != 64<<10 || !errors.Is(err, wantErr) {
		t.Fatalf("read = %d, %v; want first chunk then %v", n, err, wantErr)
	}
	if !bytes.Equal(buf[:n], bytes.Repeat([]byte{1}, n)) {
		t.Fatal("bytes preceding the failed chunk changed")
	}
}

func TestBlobReaderReadAheadSeekCancelsOldWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reader, store := newReadAheadFixture(t, ctx, 10, 64<<10)
	defer reader.Close()
	close(store.gates[0])
	buf := make([]byte, 64<<10)
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		waitChunkFetch(t, ctx, store)
	}

	if _, err := reader.Seek(9*64<<10, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if got := store.active.Load(); got != 0 {
		t.Fatalf("Seek returned with %d old reads active", got)
	}
	close(store.gates[9])
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, bytes.Repeat([]byte{10}, len(buf))) {
		t.Fatal("Seek returned old window bytes")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if got := store.calls.Load(); got != 9 {
		t.Fatalf("fetched %d chunks, want original eight and seek target", got)
	}
}

func TestBlobReaderReadAheadParentCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reader, store := newReadAheadFixture(t, ctx, 10, 64<<10)
	defer reader.Close()
	readDone := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 32<<10))
		readDone <- err
	}()
	for range 8 {
		waitChunkFetch(t, ctx, store)
	}
	cancel()
	if err := <-readDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("read error = %v, want cancellation", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if got := store.active.Load(); got != 0 {
		t.Fatalf("canceled reader retained %d active reads", got)
	}
}

// chunkFetchStore gates real stored chunks so tests observe overlapping reads
// and cancellation without relying on elapsed time for synchronization.
type chunkFetchStore struct {
	block.StoreOps
	indices map[string]int
	gates   []chan struct{}
	started chan int
	active  atomic.Int32
	calls   atomic.Int32
	failIdx int
	failErr error
}

func (s *chunkFetchStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	idx, ok := s.indices[ref.MarshalString()]
	if !ok {
		return s.StoreOps.GetBlock(ctx, ref)
	}
	s.calls.Add(1)
	s.active.Add(1)
	defer s.active.Add(-1)
	s.started <- idx
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-s.gates[idx]:
	}
	if idx == s.failIdx {
		return nil, false, s.failErr
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

func waitChunkFetch(t *testing.T, ctx context.Context, store *chunkFetchStore) int {
	t.Helper()
	select {
	case idx := <-store.started:
		return idx
	case <-ctx.Done():
		t.Fatal("expected overlapping chunk read did not start")
		return -1
	}
}

func newReadAheadFixture(t *testing.T, ctx context.Context, count, size int) (*Reader, *chunkFetchStore) {
	t.Helper()
	base := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(base, nil, nil, nil)
	root := &Blob{BlobType: BlobType_BlobType_CHUNKED, TotalSize: uint64(count * size), ChunkIndex: &ChunkIndex{}}
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

	store := &chunkFetchStore{
		StoreOps: base,
		indices:  make(map[string]int, count),
		gates:    make([]chan struct{}, count),
		started:  make(chan int, count*2),
		failIdx:  -1,
	}
	_, cursor = block.NewTransaction(store, nil, ref, nil)
	reader, err := NewReader(ctx, cursor)
	if err != nil {
		t.Fatal(err)
	}
	for idx, chunk := range reader.root.GetChunkIndex().GetChunks() {
		store.indices[chunk.GetDataRef().MarshalString()] = idx
		store.gates[idx] = make(chan struct{})
	}
	return reader, store
}
