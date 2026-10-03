package file

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
)

func TestBasicCreateRootBlob(t *testing.T) {
	// Prepare a file transaction and its test contents.
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	testBuf := []byte("test data 123")

	// Build the file with an inline root blob.
	_, err := BuildFileWithBytes(ctx, bcs, testBuf, &blob.BuildBlobOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Publish the newly created file.
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the published file root for readback.
	// root index is eves[len(eves)-1]
	_, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	fi, err := block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the newly created file through its handle.
	rdr := NewHandle(ctx, bcs, fi)
	defer rdr.Close()
	ob, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the created file contains the original bytes.
	if !bytes.Equal(ob, testBuf) {
		t.Fatalf("output mismatch: %v != %v", ob, testBuf)
	}
}
