package blob

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
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
	_ = rootRef
	rootBlobBlk, err := bcs.Unmarshal(NewBlobBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootBlob := rootBlobBlk.(*Blob)
	if err := rootBlob.ValidateFull(context.Background(), nil); err != nil {
		t.Fatal(err.Error())
	}
	opDur := t2.Sub(t1)
	t.Logf(
		"built %d byte blob with %d chunks and polynomial %v in %s (%v / sec)",
		rootBlob.GetTotalSize(),
		len(rootBlob.GetChunkIndex().GetChunks()),
		rootBlob.GetChunkIndex().GetPol(),
		opDur,
		uint64(float64(rootBlob.GetTotalSize())/opDur.Seconds()),
	)

	// Read the data back into a buffer.
	oc.SetRootRef(rootRef)
	btx, bcs = oc.BuildTransaction(nil)
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
	if len(dat) != int(rootBlob.GetTotalSize()) {
		t.Fatalf("expected to read %d but got %d", rootBlob.GetTotalSize(), len(dat))
	}
	opDur = t2.Sub(t1)
	t.Logf(
		"read and verified %d bytes in %s (%d / sec)",
		len(dat),
		opDur.String(),
		int(float64(len(dat))/opDur.Seconds()),
	)
}
