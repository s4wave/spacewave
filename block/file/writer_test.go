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

func TestBasicWriter(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader")
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile)
	eves, bcs, err := btx.Write()
	if err != nil {
		t.Fatal(err.Error())
	}
	// root index is eves[len(eves)-1]
	rootRef := eves[len(eves)-1].GetPutBlock().GetBlockCommon().GetBlockRef()
	btx, bcs = block.NewTransaction(bkt, rootRef, nil)
	fi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	testBuf := []byte("test data testing")
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := ioutil.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(ob) != 0 {
		t.Fatal("expected empty file")
	}
	writer := NewWriter(rdr, btx, blob.BuildBlobOpts{})
	n, err := writer.Write(testBuf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != len(testBuf) {
		t.Fatal("n != len(testBuf)")
	}

	w1Ref := writer.GetRef()
	btx, bcs = block.NewTransaction(bkt, w1Ref, nil)
	fi, err = bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	w1handle := NewHandle(ctx, bcs, fi.(*File))
	ob, err = ioutil.ReadAll(w1handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob, testBuf) != 0 {
		t.Fatalf("output mismatch: %v != %v", ob, testBuf)
	}
}
