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

func TestBasicWriter(t *testing.T) {
	// Create and publish an empty file fixture.
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the empty file root for writing.
	// root index is eves[len(eves)-1]
	btx, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	fi, err := block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the file handle and prepare the test bytes.
	testBuf := []byte("test data testing")
	rdr := NewHandle(ctx, bcs, fi)
	defer rdr.Close()
	ob, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the new file initially contains no bytes.
	if len(ob) != 0 {
		t.Fatal("expected empty file")
	}

	// Write the test bytes and verify the complete write count.
	writer := NewWriter(rdr, btx, &blob.BuildBlobOpts{})
	n, err := writer.Write(testBuf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != len(testBuf) {
		t.Fatal("n != len(testBuf)")
	}

	// Reopen the file root published by the writer.
	w1Ref := writer.GetRef()
	btx, bcs = block.NewTransaction(bkt, nil, w1Ref, nil)
	fi, err = block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the published file and verify its contents.
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

	// Verify readback after shrinking the file to four bytes.
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

	// Verify the extended file preserves its prefix and fills its tail with zeros.
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
	// Open a mock bucket for file append coverage.
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)

	// Create an empty file in a block transaction.
	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)

	// Write the initial file contents and close the handle.
	fh := NewHandle(ctx, bcs, rootFile)
	fw := NewWriter(fh, btx, nil)
	_ = fw.WriteBytes(0, []byte("test"))
	fh.Close()

	// Publish the initial append fixture.
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the file root for appending.
	btx, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	rootFile, err = block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Append bytes to the file through a new writer.
	fh = NewHandle(ctx, bcs, rootFile)
	fw = NewWriter(fh, btx, nil)
	err = fw.WriteBytes(4, []byte("append"))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the appended bytes extend the raw root blob.
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
	// Open a mock bucket for range-to-root folding coverage.
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)

	// Create an empty file in a block transaction.
	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)

	// Write the initial file contents and close the handle.
	fh := NewHandle(ctx, bcs, rootFile)
	fw := NewWriter(fh, btx, nil)
	_ = fw.WriteBytes(0, []byte("test"))
	fh.Close()

	// Publish the range folding fixture.
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the file root for a partial overwrite.
	btx, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	rootFile, err = block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Overwrite the end of the file and verify it creates two ranges.
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

// TestWriteStoredRawBlobKeepsReference verifies a file written from a stored raw
// blob over maxInlineRawBlobSize references it instead of copying its bytes
// into the file block.
func TestWriteStoredRawBlobKeepsReference(t *testing.T) {
	// Prepare a stored raw blob larger than the inline limit.
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-stored-raw-blob", nil)
	data := bytes.Repeat([]byte("stored raw blob "), 4096)

	// Build and publish the raw blob independently of the file.
	btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
	if _, err := blob.BuildBlobWithBytes(ctx, data, bcs); err != nil {
		t.Fatal(err.Error())
	}
	blobRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a file writer that references the stored raw blob.
	btx, bcs = block.NewTransaction(bkt, nil, nil, nil)
	rootFile := &File{}
	bcs.SetBlock(rootFile, true)
	fh := NewHandle(ctx, bcs, rootFile)
	if err := NewWriter(fh, btx, nil).WriteBlob(0, uint64(len(data)), blobRef); err != nil {
		t.Fatal(err.Error())
	}
	fh.Close()

	// Verify the large stored blob remains in one file range.
	if rootFile.GetRootBlob() != nil || len(rootFile.GetRanges()) != 1 {
		t.Fatalf("root blob = %v, ranges = %d, want one range and no root blob", rootFile.GetRootBlob() != nil, len(rootFile.GetRanges()))
	}

	// Publish the file containing the stored blob reference.
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the file root and verify its stored blob reference.
	_, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
	fi, err := block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !fi.GetRanges()[0].GetRef().EqualsRef(blobRef) {
		t.Fatal("range does not reference the stored blob")
	}

	// Read the referenced blob through the file handle.
	rdr := NewHandle(ctx, bcs, fi)
	defer rdr.Close()
	got, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify readback returns the complete stored blob contents.
	if !bytes.Equal(got, data) {
		t.Fatalf("read %d bytes, want the %d written", len(got), len(data))
	}
}

// TestTruncateRootBlobThenPartialWrite verifies that shrinking a file held in
// its root blob drops the truncated bytes so later writes do not revive them.
func TestTruncateRootBlobThenPartialWrite(t *testing.T) {
	type op struct {
		truncate bool
		off      uint64
		data     string
	}
	cases := []struct {
		name string
		ops  []op
		want string
	}{{
		name: "write past end",
		ops:  []op{{off: 0, data: "hello world"}, {truncate: true, off: 5}, {off: 7, data: "XY"}},
		want: "hello\x00\x00XY",
	}, {
		name: "write inside then extend",
		ops:  []op{{off: 0, data: "hello world"}, {truncate: true, off: 5}, {off: 2, data: "Z"}, {truncate: true, off: 11}},
		want: "heZlo\x00\x00\x00\x00\x00\x00",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create an empty file and writer for the truncation sequence.
			ctx := context.Background()
			bkt := bucket_mock.NewMockBucket("test-truncate-root-blob", nil)
			btx, bcs := block.NewTransaction(bkt, nil, nil, nil)
			rootFile := &File{}
			bcs.SetBlock(rootFile, true)
			fh := NewHandle(ctx, bcs, rootFile)
			defer fh.Close()
			fw := NewWriter(fh, btx, nil)

			// Apply each truncation or partial write in the test case.
			for _, o := range tc.ops {
				// Execute the requested file operation and require it to succeed.
				var err error
				if o.truncate {
					err = fw.Truncate(o.off)
				} else {
					err = fw.WriteBytes(o.off, []byte(o.data))
				}
				if err != nil {
					t.Fatal(err.Error())
				}
			}

			// Publish the file after the operation sequence.
			rootRef, _, err := btx.Write(ctx, true)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Reopen the published file root for readback.
			_, bcs = block.NewTransaction(bkt, nil, rootRef, nil)
			fi, err := block.UnmarshalBlock[*File](ctx, bcs, NewFileBlock)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Read the file after truncation and partial writes.
			rdr := NewHandle(ctx, bcs, fi)
			defer rdr.Close()
			got, err := io.ReadAll(rdr)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Verify later writes and extensions preserve the truncated contents.
			if string(got) != tc.want {
				t.Fatalf("read %q, want %q", got, tc.want)
			}
		})
	}
}
