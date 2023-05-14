package file

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	bucket_mock "github.com/aperturerobotics/hydra/bucket/mock"
)

func TestBasicWriter(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)
	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// root index is eves[len(eves)-1]
	btx, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	fi, err := block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	testBuf := []byte("test data testing")
	rdr := NewHandle(ctx, bcs, fi)
	defer rdr.Close()
	ob, err := io.ReadAll(rdr)
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
	btx, bcs = block.NewTransaction(bkt, nil, w1Ref, nil)
	fi, err = block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	w1handle := NewHandle(ctx, bcs, fi)
	ob, err = io.ReadAll(w1handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob, testBuf) {
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
	ob, err = io.ReadAll(w1handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob, testBuf[:4]) {
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
	ob, err = io.ReadAll(w1handle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob[:4], testBuf[:4]) {
		t.Fatalf("truncated output mismatch: %v != %v", ob[:4], testBuf[:4])
	}
	for i := 4; i < 8; i++ {
		if ob[i] != 0 {
			t.Fatalf("extended portion is not zeros: %v", ob[4:])
		}
	}
}

func TestAppend(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)

	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)

	fh := NewHandle(ctx, bcs, rootFile)
	fw := NewWriter(fh, btx, nil)
	_ = fw.WriteBytes(0, []byte("test"))
	fh.Close()

	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	rootFile, err = block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	fh = NewHandle(ctx, bcs, rootFile)
	fw = NewWriter(fh, btx, nil)
	err = fw.WriteBytes(4, []byte("append"))
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(rootFile.GetRootBlob().GetRawData()) != 10 {
		t.Fail()
	}

	// test appending to the raw blob
	err = fw.WriteBytes(fw.root.TotalSize, []byte("araw"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 0 {
		t.Fail()
	}

	// write some data out of sequence (triggering a move to ranges)
	oosWrite := []byte("foo")
	err = fw.WriteBytes(1, oosWrite)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 2 {
		t.Fail()
	}

	// append to last range (no more ranges should be made)
	err = fw.WriteBytes(fw.root.TotalSize, []byte("append"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 2 {
		t.Fail()
	}

	// extend the file, without extending a range
	err = fw.WriteBytes(fw.root.TotalSize-1, []byte("extending-the-file"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 3 {
		t.Fail()
	}

	// truncate, deleting all but 2 of the ranges
	if err := fw.Truncate(4); err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 2 {
		t.Fail()
	}
}

func TestMoveRangeToRootBlob(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)

	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)

	fh := NewHandle(ctx, bcs, rootFile)
	fw := NewWriter(fh, btx, nil)
	_ = fw.WriteBytes(0, []byte("test"))
	fh.Close()

	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	rootFile, err = block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	fh = NewHandle(ctx, bcs, rootFile)
	fw = NewWriter(fh, btx, nil)
	err = fw.WriteBytes(fw.root.TotalSize-1, []byte("append"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 2 {
		t.Fail()
	}

	// truncate, deleting all but 1 range
	if err := fw.Truncate(2); err != nil {
		t.Fatal(err.Error())
	}
	if len(rootFile.GetRanges()) != 0 {
		t.Fail()
	}
	if rootFile.GetRootBlob().GetTotalSize() != 2 {
		t.Fail()
	}
}
