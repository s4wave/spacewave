package file

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	bucket_mock "github.com/aperturerobotics/hydra/bucket/mock"
)

func TestBasicCreateRootBlob(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	testBuf := []byte("test data 123")
	rootFile, bcs, err := BuildFileWithBytes(ctx, btx, bcs, testBuf, &blob.BuildBlobOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	bcs.SetBlock(rootFile, true)
	rootRef, bcs, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// root index is eves[len(eves)-1]
	btx, bcs = block.NewTransaction(bkt, rootRef, nil)
	fi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := ioutil.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob, testBuf) != 0 {
		t.Fatalf("output mismatch: %v != %v", ob, testBuf)
	}
}
