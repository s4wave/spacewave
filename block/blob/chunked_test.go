package blob

import (
	"bytes"
	"context"
	"io"
	"math"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/util/prng"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// testBlobChunked contains the common test logic for chunked blob tests.
func testBlobChunked(t *testing.T, chunkerType string, chunkerArgs *ChunkerArgs) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs := oc.BuildTransaction(nil)
	t1 := time.Now()
	b1, err := buildMockChunkedBlob(bcs, chunkerArgs)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = b1
	/*
		if err := b1.ValidateFull(context.Background(), nil); err != nil {
			t.Fatal(err.Error())
		}
	*/
	rootRef, bcs, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t2 := time.Now()
	opDur := t2.Sub(t1)

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
	if len(dat) != int(b1.GetTotalSize()) {
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
	if bbuf.Len() != int(b1.GetTotalSize()) {
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

	bbuf.Reset()
	if err := FetchToBuffer(ctx, bcs, &bbuf); err != nil {
		t.Fatal(err.Error())
	}
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
	if b1.GetBlobType() != BlobType_BlobType_CHUNKED || b1.GetTotalSize() != uint64(truncateSize) {
		t.Fail()
	}
	fetched, err := FetchToBytes(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(fetched, expectedData[:truncateSize]) {
		t.Fail()
	}
	chunks := b1.GetChunkIndex().GetChunks()
	lastChk := chunks[len(chunks)-1]
	lastChkEnd := lastChk.GetStart() + lastChk.GetSize()
	if lastChkEnd != uint64(truncateSize) {
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
	for i := 0; i < 10000; i++ {
		// get random location
		loc := int64(prand.Uint64() % uint64(len(expectedData)))
		// read from that location
		seekPos, err := blobReader.Seek(loc, io.SeekStart)
		if err == nil && seekPos != loc {
			err = errors.Errorf("asked to seek to %d but got %d", loc, seekPos)
		}
		if err != nil {
			t.Fatal(err.Error())
		}
		n, err := blobReader.Read(buf)
		if err != nil {
			t.Fatal(err.Error())
		}
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
