package bucket_mock

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_snappy "github.com/aperturerobotics/hydra/block/transform/snappy"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// TestCursor tests the basic object cursor mechanics.
func TestCursor(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_snappy.Config{},
		&transform_chksum.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// test building with empty tconf
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		testbed.BucketId,
		volID,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.Release()

	// test with actual tconf
	oc, _, err = bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		testbed.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	txc, tcc := oc.BuildTransaction(nil)
	tcc.SetBlock(&block_mock.Root{}, true)
	tsb1 := tcc.FollowSubBlock(1)
	tcc2 := tsb1.FollowRef(1, nil)
	tcc2.SetBlock(&block_mock.Example{Msg: "hello world"}, true)
	nrb, _, err := txc.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc.SetRootRef(nrb)
	txc, tcc = oc.BuildTransaction(nil)
	tcc.SetBlock(&Root{ExamplePtr: oc.GetRef()}, true)

	nrb, _, err = txc.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Logf("root block: %s", nrb.MarshalString())
	oc.SetRootRef(nrb)

	// fetch the root out again building a whole new cursor
	ocr := oc.GetRef()
	// oct := oc.GetTransformConf()
	oc.Release()
	oc, err = bucket_lookup.BuildCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		volID,
		ocr,
		// oct,
		nil, // NOTE: The transform conf is in the reference.
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	rbi, err := oc.Unmarshal(func() block.Block { return &Root{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	rb := rbi.(*Root)
	t.Logf(
		"example pointer -> %s",
		rb.GetExamplePtr().GetRootRef().MarshalString(),
	)

	occ, err := oc.FollowRef(ctx, rb.GetExamplePtr())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer occ.Release()
	_, tcc = occ.BuildTransaction(nil)
	bmr, err := tcc.Unmarshal(func() block.Block { return &block_mock.Root{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	bm := bmr.(*block_mock.Root)

	sbcr := tcc.FollowSubBlock(1)
	tcc = sbcr.FollowRef(1, bm.GetExampleSubBlock().GetExamplePtr())
	if err != nil {
		t.Fatal(err.Error())
	}
	bmr, err = tcc.Unmarshal(block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	msg := bmr.(*block_mock.Example).GetMsg()
	if len(msg) == 0 {
		t.Fail()
	}
	t.Logf("got message from block: %s", msg)

	// in-line transform config
	encKey, _ := hex.DecodeString("9e4cd7bfb3a166e0b3aa89c5bd7dca29731d83272e52ddad011c047e41b77440")
	tconf, err = block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	nc, err := oc.FollowRef(ctx, &bucket.ObjectRef{
		TransformConf: tconf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer nc.Release()
	btx, bcs := nc.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExampleBlock(), true)
	_, _, err = btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !proto.Equal(nc.GetTransformConf(), tconf) {
		t.FailNow()
	}
}
