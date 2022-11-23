package file

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	bucket_mock "github.com/aperturerobotics/hydra/bucket/mock"
	"github.com/aperturerobotics/util/prng"
	"github.com/pkg/errors"
)

func TestBasicReader(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	testBuf := []byte("test data testing")
	rootFile := &File{
		TotalSize: uint64(len(testBuf)),
		Ranges: []*Range{{
			Start:  0,
			Length: uint64(len(testBuf)),
		}},
	}
	bcs.SetBlock(rootFile, true)
	rangeSet := NewRangeSet(&rootFile.Ranges, bcs.FollowSubBlock(4))
	_, r1cs := rangeSet.Get(0)
	r1cs = r1cs.FollowRef(4, nil)
	_, err := blob.BuildBlob(
		ctx,
		int64(len(testBuf)),
		bytes.NewReader(testBuf),
		r1cs,
		&blob.BuildBlobOpts{},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// root index is eves[len(eves)-1]
	_, bcs = block.NewTransaction(bkt, rootRef, nil)
	fi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob, testBuf) {
		t.Fatal("test buffer mismatch")
	}
}

func TestInlineRootBlobReader(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	testBuf := []byte("test data testing")
	rootFile := &File{
		TotalSize: uint64(len(testBuf)),
		RootBlob:  blob.NewRawBlob(testBuf),
	}
	bcs.SetBlock(rootFile, true)
	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// root index is eves[len(eves)-1]
	_, bcs = block.NewTransaction(bkt, rootRef, nil)
	fi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob, testBuf) {
		t.Fatal("test buffer mismatch")
	}
}

/*
	TestlMultiRangeReader tests:
	   |r3| start=50 length=10
	  | r2 | start=40 length=40

| range 1 |
*/
func TestMultiRangeReader(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil)

	r1Data := make([]byte, 100)
	r2Data := make([]byte, 40)
	r3Data := make([]byte, 10)
	rand.Read(r1Data)
	rand.Read(r2Data)
	rand.Read(r3Data)

	r2Start := 40
	r3Start := 50

	expectedOutcome := make([]byte, 100)
	copy(expectedOutcome, r1Data)
	copy(expectedOutcome[r2Start:], r2Data)
	copy(expectedOutcome[r3Start:], r3Data)

	rootFile := &File{
		TotalSize:  uint64(len(r1Data)),
		RangeNonce: 2,
		Ranges: []*Range{
			{
				Nonce:  0,
				Start:  0,
				Length: 100,
			},
			{
				Nonce:  1,
				Start:  uint64(r2Start),
				Length: uint64(len(r2Data)),
			},
			{
				Nonce:  2,
				Start:  uint64(r3Start),
				Length: uint64(len(r3Data)),
			},
		},
	}

	bcs.SetBlock(rootFile, true)
	rangeSet := NewRangeSet(&rootFile.Ranges, bcs.FollowSubBlock(4))
	buildRangeData := func(idx int, data []byte) {
		_, rncs := rangeSet.Get(idx)
		rncs = rncs.FollowRef(4, nil)
		_, err := blob.BuildBlob(
			ctx,
			int64(len(data)),
			bytes.NewReader(data),
			rncs,
			&blob.BuildBlobOpts{},
		)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	buildRangeData(0, r1Data)
	buildRangeData(1, r2Data)
	buildRangeData(2, r3Data)

	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// root index is eves[len(eves)-1]
	_, bcs = block.NewTransaction(bkt, rootRef, nil)
	fi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob, expectedOutcome) {
		t.Fatalf(
			"test buffer mismatch\nob(%d): %v\nexpected: %v\nt1: %v\nt2: %v\nt3: %v",
			len(ob),
			ob,
			expectedOutcome,
			r1Data,
			r2Data,
			r3Data,
		)
	}
}

// TestRandomReads tests random reads from a 1Mb file of random data.
func TestRandomReads(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-reader-random-reads", nil)
	btx, bcs := block.NewTransaction(bkt, nil, nil)

	expectedData := make([]byte, 1e6)
	rand.Read(expectedData)

	_, err := BuildFileWithBytes(ctx, bcs, expectedData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	rootRef, bcs, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	fi, err := UnmarshalFile(bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// sanity check: read entire file
	rdr := NewHandle(ctx, bcs, fi)
	ob, err := io.ReadAll(rdr)
	_ = rdr.Close()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(ob, expectedData) {
		t.Fatal("expected data did not match read data")
	}

	// start from scratch: random reads
	_, bcs = block.NewTransaction(bkt, rootRef, nil)
	fi, err = UnmarshalFile(bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test random reads
	rdr = NewHandle(ctx, bcs, fi)
	prand := prng.BuildSeededRand([]byte("random-reads"))
	_ = prand
	buf := make([]byte, 4096)
	for i := 0; i < 10000; i++ {
		// get random location (fails)
		loc := int64(prand.Float32() * float32(len(expectedData)))
		// sequential: works perfectly
		// loc := int64(i * 4096)
		if int(loc) >= len(expectedData) {
			break
		}
		// read from that location
		seekPos, err := rdr.Seek(loc, io.SeekStart)
		if err == nil && seekPos != loc {
			err = errors.Errorf("asked to seek to %d but got %d", loc, seekPos)
		}
		if err != nil {
			t.Fatal(err.Error())
		}
		n, err := rdr.Read(buf)
		if err != nil {
			t.Fatal(err.Error())
		}
		readData := buf[:n]
		readExpected := expectedData[loc : int(loc)+n]
		if !bytes.Equal(readExpected, readData) {
			t.Fatalf("read incorrect data n(%d) @ %d: len(%d): %v... != expected %v...", i, loc, n, readData[:12], readExpected[:12])
		}
	}

}
