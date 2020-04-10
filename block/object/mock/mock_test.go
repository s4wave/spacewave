package object_mock

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/block/object"
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/block/transform/snappy"
	"github.com/aperturerobotics/hydra/testbed"
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
	oc, _, err := object.BuildEmptyCursor(
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

	// test with actual tconf
	oc, _, err = object.BuildEmptyCursor(
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
	tcc.SetBlock(&block_mock.Root{})
	tsb1 := tcc.FollowSubBlock(1)
	tcc2 := tsb1.FollowRef(1, nil)
	tcc2.SetBlock(&block_mock.Example{Msg: "hello world"})
	eves, _, err := txc.Write()
	if err != nil {
		t.Fatal(err.Error())
	}
	nrb := eves[len(eves)-1].GetPutBlock().GetBlockCommon().GetBlockRef()

	oc.SetRootRef(nrb)
	txc, tcc = oc.BuildTransaction(nil)
	tcc.SetBlock(&Root{ExamplePtr: oc.GetRef()})
	eves, _, err = txc.Write()
	if err != nil {
		t.Fatal(err.Error())
	}

	nrb = eves[len(eves)-1].GetPutBlock().GetBlockCommon().GetBlockRef()
	t.Logf("root block: %s", nrb.MarshalString())
	oc.SetRootRef(nrb)

	// fetch the root out again building a whole new cursor
	ocr := oc.GetRef()
	// oct := oc.GetTransformConf()
	oc.Release()
	oc, err = object.BuildCursor(
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
	txc, tcc = occ.BuildTransaction(nil)
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
}
