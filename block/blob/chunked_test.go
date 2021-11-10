package blob

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/dustin/go-humanize"
	"github.com/sirupsen/logrus"
)

// TestBlob_Chunked tests building a chunked blob.
func TestBlob_Chunked(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs := oc.BuildTransaction(nil)
	t1 := time.Now()
	b1, err := buildMockChunkedBlob(bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = b1
	/*
		if err := b1.ValidateFull(context.Background(), nil); err != nil {
			t.Fatal(err.Error())
		}
	*/
	rootRef, bcs, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t2 := time.Now()
	opDur := t2.Sub(t1)

	_ = rootRef
	rootBlobBlk, err := bcs.Unmarshal(NewBlobBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	b1 = rootBlobBlk.(*Blob)
	if err := b1.ValidateFull(context.Background(), nil); err != nil {
		t.Fatal(err.Error())
	}
	t.Logf(
		"built & wrote %s blob with %d chunks in %s (%v / sec)",
		humanize.Bytes(b1.GetTotalSize()),
		len(b1.GetChunkIndex().GetChunks()),
		opDur,
		humanize.Bytes(uint64(float64(b1.GetTotalSize())/opDur.Seconds())),
	)

	// Read the data back into a buffer.
	oc.SetRootRef(rootRef)
	btx, bcs = oc.BuildTransaction(nil)
	rootBlobData, _, _ := bcs.Fetch()
	rootBlobSize := uint64(len(rootBlobData))
	t.Logf(
		"index block is %s (overhead of %v%%)",
		humanize.Bytes(rootBlobSize),
		uint64(float64(rootBlobSize)/float64(b1.GetTotalSize())*100),
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
	t.Logf(
		"read and verified %s bytes in %s (%s / sec)",
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
	rootBlobBlk, err = bcs.Unmarshal(NewBlobBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	b1 = rootBlobBlk.(*Blob)

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

	// TODO: check appending to a raw blob
}
