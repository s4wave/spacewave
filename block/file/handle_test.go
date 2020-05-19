package file

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/bucket/mock"
)

func TestBasicReader(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader")
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	testBuf := []byte("test data testing")
	rootFile := &File{
		TotalSize: uint64(len(testBuf)),
		Ranges: []*Range{
			&Range{
				Start:  0,
				Length: uint64(len(testBuf)),
			},
		},
	}
	bcs.SetBlock(rootFile)
	r1cs := bcs.FollowRef(NewFileRangeRefId(0), nil)
	_, err := blob.BuildBlob(
		ctx,
		uint64(len(testBuf)),
		bytes.NewReader(testBuf),
		r1cs,
		&blob.BuildBlobOpts{},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
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
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := ioutil.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob, testBuf) != 0 {
		t.Fatal("test buffer mismatch")
	}
}

func TestInlineRootBlobReader(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader")
	btx, bcs := block.NewTransaction(bkt, nil, nil)
	testBuf := []byte("test data testing")
	rootFile := &File{
		TotalSize: uint64(len(testBuf)),
		RootBlob:  blob.NewRawBlob(testBuf),
	}
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
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := ioutil.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob, testBuf) != 0 {
		t.Fatal("test buffer mismatch")
	}
}

/* TestMultiRangeReader tests:
    |r3| start=50 length=10
   | r2 | start=40 length=40
| range 1 |
*/
func TestMultiRangeReader(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("test-basic-reader")
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
			&Range{
				Nonce:  0,
				Start:  0,
				Length: 100,
			},
			&Range{
				Nonce:  1,
				Start:  uint64(r2Start),
				Length: uint64(len(r2Data)),
			},
			&Range{
				Nonce:  2,
				Start:  uint64(r3Start),
				Length: uint64(len(r3Data)),
			},
		},
	}
	bcs.SetBlock(rootFile)

	buildRangeData := func(idx int, data []byte) {
		rncs := bcs.FollowRef(NewFileRangeRefId(idx), nil)
		_, err := blob.BuildBlob(
			ctx,
			uint64(len(data)),
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
	rdr := NewHandle(ctx, bcs, fi.(*File))
	defer rdr.Close()
	ob, err := ioutil.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(ob, expectedOutcome) != 0 {
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
