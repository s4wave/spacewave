package e2e_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
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
	bc := &bucket.Config{Id: "test-bucket", Rev: 1}
	applyBc := func(tb *testbed.Testbed) {
		_, err := bucket.ExApplyBucketConfig(
			tb.Context,
			tb.Bus,
			bucket.NewApplyBucketConfigToVolume(
				bc,
				tb.Volume.GetID(),
			),
		)
		if err != nil {
			t.Fatal(err.Error())
		}
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
	}, transform_blockenc.NewStepFactory().ConstructMockConfig()...))
	if err != nil {
		t.Fatal(err.Error())
	}
	assertDataWriteRead := func(tb *testbed.Testbed, dataXfer []byte) {
		applyBc(tb)
		rootCursor, _, err := bucket_lookup.BuildEmptyCursor(
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

		dataXferRef, _, err := rootCursor.PutBlock(tb.Context, dataXfer, nil)
		if err != nil {
			t.Fatal(err.Error())
		}

		tb.Logger.Infof(
			"placed block in first bucket with ref %s",
			dataXferRef.MarshalString(),
		)

		lkDat, lkOk, err := rootCursor.GetBlock(tb.Context, dataXferRef)
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
		func(t *testing.T, tb *testbed.Testbed) {
			assertDataWriteRead(tb, randData(27))
		},
	)
	testbed.RunSubtest(
		t,
		"encode-with-extend",
		func(t *testing.T, tb *testbed.Testbed) {
			// this should extend the slice without re-alloc
			x := make([]byte, 38)
			copy(x, randData(27))
			x = x[:27]
			assertDataWriteRead(tb, x)
		},
	)
}
