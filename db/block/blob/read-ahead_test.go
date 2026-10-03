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
	// Exercise read-ahead limits for concurrency, bytes, and oversized chunks.
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
			// Open a bounded reader fixture whose chunk fetches can be observed.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			reader, store := newReadAheadFixture(t, ctx, tc.count, tc.chunkSize)
			defer reader.Close()

			// Start a prefix read while the fixture holds chunk fetches behind gates.
			buf := make([]byte, 32<<10)
			readDone := make(chan error, 1)
			go func() {
				_, err := io.ReadFull(reader, buf)
				readDone <- err
			}()

			// Observe the expected read-ahead window and verify every required chunk starts.
			seen := make(map[int]bool)
			for range tc.wantFetches {
				seen[waitChunkFetch(t, ctx, store)] = true
			}
			for idx := range tc.wantFetches {
				if !seen[idx] {
					t.Fatalf("window omitted chunk %d: %v", idx, seen)
				}
			}

			// Release the demanded chunk and wait for the prefix read to complete.
			close(store.gates[0])
			if err := <-readDone; err != nil {
				t.Fatal(err)
			}

			// Verify the demanded chunk bytes remain unchanged.
			if !bytes.Equal(buf, bytes.Repeat([]byte{1}, len(buf))) {
				t.Fatal("first chunk bytes changed")
			}

			// Close the reader and verify no fetch remains beyond the bounded window.
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
	// Open a gated reader fixture for a read smaller than the read-ahead threshold.
	reader, store := newReadAheadFixture(t, t.Context(), 3, 64<<10)
	defer reader.Close()

	// Read a small prefix with only the first chunk gate open.
	close(store.gates[0])
	if _, err := io.ReadFull(reader, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify the small read fetches exactly one chunk.
	if got := store.calls.Load(); got != 1 {
		t.Fatalf("small read fetched %d chunks, want 1", got)
	}
}

func TestBlobReaderReadAheadPreservesErrorOrder(t *testing.T) {
	// Prepare a reader fixture whose second chunk returns a known error.
	reader, store := newReadAheadFixture(t, t.Context(), 3, 64<<10)
	defer reader.Close()
	wantErr := errors.New("unavailable second chunk")
	store.failIdx, store.failErr = 1, wantErr
	for _, gate := range store.gates {
		close(gate)
	}

	// Read across the failing chunk through the blob reader.
	buf := make([]byte, 3*64<<10)
	n, err := io.ReadFull(reader, buf)

	// Verify the reader returns the first chunk before reporting the next error.
	if n != 64<<10 || !errors.Is(err, wantErr) {
		t.Fatalf("read = %d, %v; want first chunk then %v", n, err, wantErr)
	}
	if !bytes.Equal(buf[:n], bytes.Repeat([]byte{1}, n)) {
		t.Fatal("bytes preceding the failed chunk changed")
	}
}

func TestBlobReaderReadAheadSeekCancelsOldWindow(t *testing.T) {
	// Open a gated reader fixture for observing read-ahead cancellation on seek.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reader, store := newReadAheadFixture(t, ctx, 10, 64<<10)
	defer reader.Close()

	// Read the first chunk and observe the original eight-chunk window.
	close(store.gates[0])
	buf := make([]byte, 64<<10)
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		waitChunkFetch(t, ctx, store)
	}

	// Seek beyond the original read-ahead window.
	if _, err := reader.Seek(9*64<<10, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	// Verify the seek cancels every active read from the original window.
	if got := store.active.Load(); got != 0 {
		t.Fatalf("Seek returned with %d old reads active", got)
	}

	// Read the seek target after releasing its chunk gate.
	close(store.gates[9])
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatal(err)
	}

	// Verify the seek returns bytes from the target chunk.
	if !bytes.Equal(buf, bytes.Repeat([]byte{10}, len(buf))) {
		t.Fatal("Seek returned old window bytes")
	}

	// Close the reader and verify only the old window and target were fetched.
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if got := store.calls.Load(); got != 9 {
		t.Fatalf("fetched %d chunks, want original eight and seek target", got)
	}
}

func TestBlobReaderReadAheadParentCancellation(t *testing.T) {
	// Open a reader fixture whose parent context can cancel active fetches.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reader, store := newReadAheadFixture(t, ctx, 10, 64<<10)
	defer reader.Close()

	// Start a blob read and observe all fetches in its read-ahead window.
	readDone := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 32<<10))
		readDone <- err
	}()
	for range 8 {
		waitChunkFetch(t, ctx, store)
	}

	// Cancel the parent context and verify the pending read reports cancellation.
	cancel()
	if err := <-readDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("read error = %v, want cancellation", err)
	}

	// Close the canceled reader and verify all active fetches have stopped.
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
	// Delegate block references outside the gated chunk fixture to the backing store.
	idx, ok := s.indices[ref.MarshalString()]
	if !ok {
		return s.StoreOps.GetBlock(ctx, ref)
	}

	// Count the chunk fetch and wait for its gate or context cancellation.
	s.calls.Add(1)
	s.active.Add(1)
	defer s.active.Add(-1)
	s.started <- idx
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-s.gates[idx]:
	}

	// Return the configured chunk failure after the fetch gate opens.
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
	// Build a persisted chunked blob with recognizable bytes in each chunk.
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

	// Persist the fixture chunks before opening the gated reader.
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the persisted blob through a store with observable chunk fetches.
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

	// Bind each persisted chunk reference to its fetch gate.
	for idx, chunk := range reader.root.GetChunkIndex().GetChunks() {
		store.indices[chunk.GetDataRef().MarshalString()] = idx
		store.gates[idx] = make(chan struct{})
	}
	return reader, store
}
