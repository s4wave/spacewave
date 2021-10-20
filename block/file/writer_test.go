package file

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	bucket_mock "github.com/aperturerobotics/hydra/bucket/mock"
)

func TestBasicWriter(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	rootFile := &File{}
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
	writer := NewWriter(rdr, btx, &blob.BuildBlobOpts{})
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

	// truncate down to 4 characters - "test"
	writer = NewWriter(w1handle, btx, nil)
	err = writer.Truncate(4)
	if err == nil {
		_, err = w1handle.Seek(0, io.SeekStart)
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	ob, err = ioutil.ReadAll(w1handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob, testBuf[:4]) != 0 {
		t.Fatalf("truncated output mismatch: %v != %v", ob, testBuf[:4])
	}

	// truncate to extend file len back up to 8 characters.
	// expect the last 4 to be zeros
	err = writer.Truncate(8)
	if err == nil {
		_, err = w1handle.Seek(0, io.SeekStart)
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	ob, err = ioutil.ReadAll(w1handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob[:4], testBuf[:4]) != 0 {
		t.Fatalf("truncated output mismatch: %v != %v", ob[:4], testBuf[:4])
	}
	for i := 4; i < 8; i++ {
		if ob[i] != 0 {
			t.Fatalf("extended portion is not zeros: %v", ob[4:])
		}
	}
}
