package blob

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket/mock"
)

// TestRawBlobValidateFull tests validating a raw blob.
func TestRawBlobValidateFull(t *testing.T) {
	testBuf := []byte("test-raw-blob-validate-full")
	b1 := &Blob{
		BlobType:  BlobType_BlobType_RAW,
		TotalSize: uint32(len(testBuf)),
		RawData:   testBuf,
	}
	if err := b1.ValidateFull(context.Background(), nil); err != nil {
		t.Fatal(err.Error())
	}

	b2 := &Blob{
		BlobType:  BlobType_BlobType_RAW,
		TotalSize: uint32(len(testBuf) - 2),
		RawData:   testBuf,
	}
	if err := b2.ValidateFull(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
}

// TestRawBlobFetch tests fetching a raw blob.
func TestRawBlobFetch(t *testing.T) {
	testBuf := []byte("test-raw-blob-validate-full")
	b1 := &Blob{
		BlobType:  BlobType_BlobType_RAW,
		TotalSize: uint32(len(testBuf)),
		RawData:   testBuf,
	}
	if err := b1.ValidateFull(context.Background(), nil); err != nil {
		t.Fatal(err.Error())
	}
	mbkt := bucket_mock.NewMockBucket("test")
	_, bcs := block.NewTransaction(mbkt, nil, nil)
	bcs.SetBlock(b1)
	dat, err := FetchToBytes(context.Background(), bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	if bytes.Compare(dat, testBuf) != 0 {
		t.Fail()
	}
}
