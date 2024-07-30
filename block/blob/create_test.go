package blob

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/util/prng"
	"github.com/sirupsen/logrus"
)

// TestBuildBlobWithBytes tests building a Blob from a byte slice.
func TestBuildBlobWithBytes(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	cs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	data := []byte("hello world 1234")

	btx, bcs := cs.BuildTransactionAtRef(nil, nil)
	_, err = BuildBlobWithBytes(ctx, data, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	bref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("blob written to %s", bref.MarshalString())

	cs.SetRootRef(bref)
	_, bcs = cs.BuildTransaction(nil)
	fetched, err := FetchToBytes(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(fetched, data) {
		t.Fatalf("mismatch of fetched data: %#v != expected %#v", fetched, data)
	}

	_, bcs = cs.BuildTransaction(nil)
	b1, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}
}

// TestBuildBlobWithReader tests building a Blob from a reader w/o known size.
func TestBuildBlobWithReader(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	cs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	buildReader := func() io.Reader {
		return prng.BuildSeededReader([]byte("test-chunk-blob"))
	}

	// Test with data less than high water mark
	_, bcs := cs.BuildTransactionAtRef(nil, nil)
	builtBlob, err := BuildBlobWithReader(
		ctx,
		io.LimitReader(buildReader(), DefRawHighWaterMark-2),
		bcs,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if builtBlob.GetBlobType() != BlobType_BlobType_RAW {
		t.Fatalf("Expected raw blob but got %v", builtBlob.GetBlobType().String())
	}

	// Test with data more than high water mark
	chunkedData := make([]byte, DefRawHighWaterMark*2)
	if _, err := io.ReadAtLeast(buildReader(), chunkedData, len(chunkedData)); err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := cs.BuildTransactionAtRef(nil, nil)
	builtBlob, err = BuildBlobWithReader(
		ctx,
		io.LimitReader(buildReader(), int64(len(chunkedData))),
		bcs,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if builtBlob.GetBlobType() != BlobType_BlobType_CHUNKED {
		t.Fatalf("Expected chunked blob but got %v", builtBlob.GetBlobType().String())
	}
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	cs.SetRootRef(ref)
	_, bcs = cs.BuildTransaction(nil)
	b1, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}
	rdr, err := NewReader(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	readData, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !bytes.Equal(readData, chunkedData) {
		t.Fatal("mismatch of read data from chunked test")
	}
}
