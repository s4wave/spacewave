package blob

import (
	"bytes"
	"context"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/prng"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// testBlobChunked contains the common test logic for chunked blob tests.
func testBlobChunked(t *testing.T, chunkerType string, chunkerArgs *ChunkerArgs) {
	// Prepare the context and logger for the chunked blob fixture.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for chunked blob operations.
	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty object cursor for the blob transaction.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the chunked blob and start measuring its write duration.
	btx, bcs := oc.BuildTransaction(nil)
	t1 := time.Now()
	b1, err := buildMockChunkedBlob(bcs, chunkerArgs)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = b1

	// Persist the chunked blob and measure the completed write.
	rootRef, bcs, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t2 := time.Now()
	opDur := t2.Sub(t1)

	// Reload and validate the persisted chunked blob metadata.
	b1, err = UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := b1.ValidateFull(context.Background(), bcs); err != nil {
		t.Fatal(err.Error())
	}
	le.Infof(
		"[%s] built & wrote %s blob with %d chunks in %s (%v / sec)",
		chunkerType,
		humanize.Bytes(b1.GetTotalSize()),
		len(b1.GetChunkIndex().GetChunks()),
		opDur,
		humanize.Bytes(uint64(float64(b1.GetTotalSize())/opDur.Seconds())),
	)

	// Read the data back into a buffer.
	oc.SetRootRef(rootRef)
	_, bcs = oc.BuildTransaction(nil)
	rootBlobData, _, _ := bcs.Fetch(ctx)
	rootBlobSize := uint64(len(rootBlobData))
	le.Infof(
		"[%s] index block is %s (overhead of %v%%)",
		chunkerType,
		humanize.Bytes(rootBlobSize),
		math.Ceil(float64(rootBlobSize)/float64(b1.GetTotalSize())*100000)/1000,
	)

	// Read the persisted blob through its reader and measure the duration.
	rdr, err := NewReader(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	t1 = time.Now()
	dat, err := io.ReadAll(rdr)
	t2 = time.Now()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the reader returns the declared blob length and report throughput.
	if len(dat) != int(b1.GetTotalSize()) { //nolint:gosec
		t.Fatalf("expected to read %d but got %d", b1.GetTotalSize(), len(dat))
	}
	opDur = t2.Sub(t1)
	le.Infof(
		"[%s] read and verified %s bytes in %s (%s / sec)",
		chunkerType,
		humanize.Bytes(uint64(len(dat))),
		opDur.String(),
		humanize.Bytes(uint64(float64(len(dat))/opDur.Seconds())),
	)

	// test fetching to buffer
	var bbuf bytes.Buffer
	if err := FetchToBuffer(ctx, bcs, &bbuf); err != nil {
		t.Fatal(err.Error())
	}

	// Verify buffer fetching returns the declared blob length.
	if bbuf.Len() != int(b1.GetTotalSize()) { //nolint:gosec
		t.Fail()
	}

	// build the blob again to do the append test
	btx, bcs = oc.BuildTransactionAtRef(nil, bcs.GetRef())
	b1, err = UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test extending the chunk set
	oldData := bbuf.Bytes()
	nextData := []byte("-appended-data-to-blob")
	err = b1.AppendData(ctx, int64(len(nextData)), bytes.NewReader(nextData), bcs, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// ensure result is correct
	expectedData := make([]byte, len(oldData)+len(nextData))
	copy(expectedData, oldData)
	copy(expectedData[len(oldData):], nextData)

	// Fetch the appended blob into a fresh buffer.
	bbuf.Reset()
	if err := FetchToBuffer(ctx, bcs, &bbuf); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the appended blob matches its expected length and contents.
	if bbuf.Len() != len(expectedData) {
		t.Fail()
	}
	if !bytes.Equal(bbuf.Bytes(), expectedData) {
		t.Fail()
	}

	// write
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test converting chunked to raw
	if err := b1.TransformToRaw(ctx, bcs, b1.GetTotalSize()); err != nil {
		t.Fatal(err.Error())
	}

	// Verify conversion preserves the expected raw blob contents.
	if b1.GetBlobType() != BlobType_BlobType_RAW {
		t.Fail()
	}
	if !bytes.Equal(b1.GetRawData(), expectedData) {
		t.Fail()
	}

	// build a new cursor to test truncating
	_, bcs = oc.BuildTransactionAtRef(nil, bcs.GetRef())
	b1, err = UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// truncate to chunked blob with several chunks
	truncateSize := int(DefRawHighWaterMark + 10)
	if err := b1.Truncate(ctx, bcs, nil, int64(truncateSize)); err != nil {
		t.Fatal(err.Error())
	}

	// Verify truncation keeps the required chunked blob type and size.
	if b1.GetBlobType() != BlobType_BlobType_CHUNKED || b1.GetTotalSize() != uint64(truncateSize) { //nolint:gosec
		t.Fail()
	}

	// Read the truncated blob to verify its contents and final chunk boundary.
	fetched, err := FetchToBytes(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(fetched, expectedData[:truncateSize]) {
		t.Fail()
	}

	// Verify the last chunk ends at the truncation offset and validate the index.
	chunks := b1.GetChunkIndex().GetChunks()
	lastChk := chunks[len(chunks)-1]
	lastChkEnd := lastChk.GetStart() + lastChk.GetSize()
	if lastChkEnd != uint64(truncateSize) { //nolint:gosec
		t.Fail()
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}

	// truncate to raw blob
	truncateSize = 10
	if err := b1.Truncate(ctx, bcs, nil, int64(truncateSize)); err != nil {
		t.Fatal(err.Error())
	}

	// Verify raw truncation preserves the expected prefix and valid metadata.
	if b1.GetBlobType() != BlobType_BlobType_RAW || len(b1.GetRawData()) != truncateSize {
		t.Fail()
	}
	if !bytes.Equal(b1.GetRawData(), expectedData[:truncateSize]) {
		t.Fail()
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}

	// build cursor again
	_, bcs = oc.BuildTransactionAtRef(nil, bcs.GetRef())
	blobReader, err := NewReader(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test random reads from the ~1Mb blob.
	// this exercises seeking to different locations in a blob.
	prand := prng.BuildSeededRand([]byte("random-reads"))
	buf := make([]byte, 4096)
	for range 10000 {
		// get random location
		loc := int64(prand.Uint64() % uint64(len(expectedData))) //nolint:gosec

		// read from that location
		seekPos, err := blobReader.Seek(loc, io.SeekStart)
		if err == nil && seekPos != loc {
			err = errors.Errorf("asked to seek to %d but got %d", loc, seekPos)
		}
		if err != nil {
			t.Fatal(err.Error())
		}

		// Read the blob bytes at the randomly selected position.
		n, err := blobReader.Read(buf)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Verify the random read matches the expected blob slice.
		readData := buf[:n]
		readExpected := expectedData[loc : int(loc)+n]
		if !bytes.Equal(readExpected, readData) {
			t.Fatalf("read data len(%d) @ %d: %v != expected %v", n, loc, readData, readExpected)
		}
	}

	// test compute storage size
	storageSize, totalSize, err := blobReader.root.ComputeStorageSize(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("[%s] storage size: %d total size: %d", chunkerType, storageSize, totalSize)
}

// TestBlob_ChunkedRabin tests building a chunked blob with Rabin chunker.
func TestBlob_ChunkedRabin(t *testing.T) {
	chunkerArgs := &ChunkerArgs{
		ChunkerType: ChunkerType_ChunkerType_RABIN,
		RabinArgs: &RabinArgs{
			Pol: 13388372929173625,
		},
	}
	testBlobChunked(t, "Rabin", chunkerArgs)
}

// TestBlob_ChunkedJC tests building a chunked blob with JC chunker.
func TestBlob_ChunkedJC(t *testing.T) {
	chunkerArgs := &ChunkerArgs{
		ChunkerType: ChunkerType_ChunkerType_JC,
	}
	testBlobChunked(t, "JC", chunkerArgs)
}

func TestBlobReaderDoesNotCacheChunkData(t *testing.T) {
	// Prepare the context for a sequential chunk cache check.
	ctx := context.Background()

	// Build and persist a multi-chunk blob on the mock store.
	store := block_mock.NewMockStore(0)
	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	body := bytes.Repeat([]byte("abcd"), 512)
	chunkerArgs := &ChunkerArgs{
		ChunkerType: ChunkerType_ChunkerType_JC,
		JcArgs: &JcArgs{
			ChunkingMinSize:    64,
			ChunkingTargetSize: 128,
			ChunkingMaxSize:    256,
		},
	}
	if _, err := BuildBlob(
		ctx,
		int64(len(body)),
		bytes.NewReader(body),
		bcs,
		&BuildBlobOpts{
			RawHighWaterMark: 1,
			ChunkerArgs:      chunkerArgs,
		},
	); err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read a prefix through a fresh transaction on the persisted blob.
	_, readCursor := block.NewTransaction(store, nil, rootRef, nil)
	rdr, err := NewReader(ctx, readCursor)
	if err != nil {
		t.Fatal(err.Error())
	}
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rdr, buf); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the sequential read returns the expected prefix bytes.
	if !bytes.Equal(buf, body[:len(buf)]) {
		t.Fatalf("read prefix %q, want %q", buf, body[:len(buf)])
	}

	// Verify sequential reads leave the chunk data cursor uncached.
	chunkSet := rdr.root.GetChunkIndex().GetChunkSet(readCursor.FollowSubBlock(4))
	_, firstChunkCursor := chunkSet.Get(0)
	dataCursor := firstChunkCursor.GetExistingRef(1)
	if dataCursor == nil {
		return
	}
	if blk, _ := dataCursor.GetBlock(); blk != nil {
		t.Fatalf("sequential blob read cached chunk data block of type %T", blk)
	}
}

func TestBlobReaderReusesCurrentChunkData(t *testing.T) {
	// Prepare the context for measuring repeated reads within a chunk.
	ctx := context.Background()

	// Choose read buffers smaller than the fixture chunks.
	const readBufferSize = 32 * 1024
	const chunkSize = DefChunkingTargetSize

	// Create the mock store and generate distinct bytes for each chunk.
	baseStore := block_mock.NewMockStore(0)
	xfrm := passthroughTransform{}
	btx, bcs := block.NewTransaction(baseStore, xfrm, nil, nil)
	body := make([]byte, chunkSize*3)
	var seed uint32 = 1
	for i := range body {
		seed = seed*1664525 + 1013904223
		body[i] = byte(seed >> 24)
	}

	// Build and persist the fixture with chunks larger than the read buffer.
	chunkerArgs := &ChunkerArgs{
		ChunkerType: ChunkerType_ChunkerType_JC,
		JcArgs: &JcArgs{
			ChunkingMinSize:    chunkSize - 1,
			ChunkingTargetSize: chunkSize,
			ChunkingMaxSize:    chunkSize + 1,
		},
	}
	if _, err := BuildBlob(
		ctx,
		int64(len(body)),
		bytes.NewReader(body),
		bcs,
		&BuildBlobOpts{
			RawHighWaterMark: 1,
			ChunkerArgs:      chunkerArgs,
		},
	); err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a fresh blob reader through the counting block store.
	countStore := &getBlockCountingStore{StoreOps: baseStore}
	_, readCursor := block.NewTransaction(countStore, xfrm, rootRef, nil)
	rdr, err := NewReader(ctx, readCursor)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Validate that the fixture has multiple chunks with unique data references.
	chunks := rdr.root.GetChunkIndex().GetChunks()
	if len(chunks) < 2 {
		t.Fatalf("expected multi-chunk fixture, got %d chunk(s)", len(chunks))
	}
	dataRefs := make([]string, 0, len(chunks))
	seenRefs := make(map[string]int, len(chunks))
	chunksLargerThanRead := 0
	for i, chunk := range chunks {
		// Count chunks that require multiple reader calls.
		if chunk.GetSize() > readBufferSize {
			chunksLargerThanRead++
		}

		// Require each chunk to have a populated and unique data reference.
		ref := chunk.GetDataRef()
		if ref == nil {
			t.Fatalf("chunk %d has nil data ref", i)
		}
		key := ref.MarshalString()
		if key == "" {
			t.Fatalf("chunk %d has empty data ref", i)
		}
		if prev, ok := seenRefs[key]; ok {
			t.Fatalf("chunk %d reuses data ref from chunk %d; fixture must use unique data refs", i, prev)
		}

		// Retain each chunk reference for the fetch-count assertions.
		seenRefs[key] = i
		dataRefs = append(dataRefs, key)
	}
	if chunksLargerThanRead == 0 {
		t.Fatalf("expected at least one chunk larger than %d-byte read buffer", readBufferSize)
	}

	// Clear fixture-validation fetches before measuring sequential reads.
	countStore.reset()

	// Read the entire blob using buffers smaller than its chunks.
	var out bytes.Buffer
	buf := make([]byte, readBufferSize)
	for {
		n, err := rdr.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err.Error())
		}
		if n == 0 {
			t.Fatal("reader returned no data and no error")
		}
	}

	// Verify sequential reads preserve the complete fixture contents.
	got := out.Bytes()
	if len(got) != len(body) {
		t.Fatalf("sequential read returned %d bytes, want %d", len(got), len(body))
	}
	if !bytes.Equal(got, body) {
		for i := range body {
			if got[i] != body[i] {
				t.Fatalf("sequential read byte %d = %d, want %d", i, got[i], body[i])
			}
		}
		t.Fatal("sequential read bytes differ")
	}

	// Verify every chunk data reference was fetched exactly once.
	for i, ref := range dataRefs {
		if got := countStore.get(ref); got != 1 {
			t.Fatalf("chunk %d data ref fetched %d times, want 1", i, got)
		}
	}
}

type passthroughTransform struct{}

func (passthroughTransform) EncodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

func (passthroughTransform) DecodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

func (passthroughTransform) DecodedBlockCacheTransformKey() string {
	return "db/block/blob.passthroughTransform"
}

type getBlockCountingStore struct {
	block.StoreOps

	mtx   sync.Mutex
	reads map[string]int
}

func (s *getBlockCountingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Count the requested block reference while holding the store lock.
	key := ref.MarshalString()
	s.mtx.Lock()
	if s.reads == nil {
		s.reads = make(map[string]int)
	}
	s.reads[key]++
	s.mtx.Unlock()

	return s.StoreOps.GetBlock(ctx, ref)
}

func (s *getBlockCountingStore) reset() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.reads = make(map[string]int)
}

func (s *getBlockCountingStore) get(ref string) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.reads[ref]
}
