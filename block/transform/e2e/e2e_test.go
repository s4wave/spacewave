package e2e_test

import (
	"bytes"
	"math/rand"
	"regexp"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block/object"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/testbed"
)

// randData returns a random data sample.
func randData(l int) []byte {
	m := make([]byte, l)
	rand.Read(m)
	return m
}

// TestEncodeDecode tests encoding and decoding a block, particularly w/ padding
// case coverage.
func TestEncodeDecode(t *testing.T) {
	bc := &bucket.Config{Id: "test-bucket", Version: 1}
	applyBc := func(tb *testbed.Testbed) {
		// apply bucket config
		_, bcRef, err := bus.ExecOneOff(
			tb.Context,
			tb.Bus,
			bucket.NewApplyBucketConfig(
				bc,
				regexp.MustCompile(regexp.QuoteMeta(tb.Volume.GetID())),
			), nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		bcRef.Release()
	}
	/*
		getBucketAPI := func(tb *testbed.Testbed) bucket.Bucket {
			targetVolID := tb.Volume.GetID()
			av, avRel, err := bus.ExecOneOff(
				tb.Context,
				tb.Bus,
				volume.NewBuildBucketAPI(bc.GetId(), targetVolID),
				nil,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			avRel.Release()
			return av.GetValue().(volume.BuildBucketAPIValue).GetBucket()
		}
	*/

	tconf, err := block_transform.NewConfig(append([]config.Config{
		&transform_chksum.Config{},
	}, transform_blockenc.NewFactory().ConstructMockConfig()...))
	if err != nil {
		t.Fatal(err.Error())
	}
	assertDataWriteRead := func(tb *testbed.Testbed, dataXfer []byte) {
		applyBc(tb)
		rootCursor, _, err := object.BuildEmptyCursor(
			tb.Context,
			tb.Bus,
			tb.Logger,
			tb.StepFactorySet,
			bc.GetId(),
			tb.Volume.GetID(),
			tconf,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer rootCursor.Release()
		bpevent, err := rootCursor.GetEncBucket().PutBlock(dataXfer, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		dataXferRef := bpevent.GetBlockCommon().GetBlockRef()

		tb.Logger.Infof(
			"placed block in first bucket with ref %s",
			dataXferRef.MarshalString(),
		)

		lkDat, lkOk, err := rootCursor.GetEncBucket().GetBlock(dataXferRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !lkOk {
			t.Fatal("lookup on node 3 returned ok=false")
		}
		if len(lkDat) != len(dataXfer) || !bytes.Equal(lkDat, dataXfer) {
			t.Fatalf("data mismatch %v != %v (expected)", lkDat, dataXfer)
		}
		rootCursor.Release()
	}

	testbed.RunSubtest(
		t,
		"encode-with-append",
		func(tb *testbed.Testbed) {
			assertDataWriteRead(tb, randData(27))
		},
	)
	testbed.RunSubtest(
		t,
		"encode-with-extend",
		func(tb *testbed.Testbed) {
			// this should extend the slice without re-alloc
			x := make([]byte, 38)
			copy(x, randData(27))
			x = x[:27]
			assertDataWriteRead(tb, x)
		},
	)
}
