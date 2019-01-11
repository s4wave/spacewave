package block_mock

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// TestTransaction tests the basic transaction mechanics.
func TestTransaction(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, nref, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(
			&node_controller.Config{},
		),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer nref.Release()

	vol := tb.Volume
	volID := vol.GetID()

	// store the bucket
	bucketID := "test-bucket-1"
	_, _, bc, err := vol.PutBucketConfig(&bucket.Config{
		Id:      bucketID,
		Version: 1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(volID)
	_ = bc

	bk, bhRel, err := node.StartBucketRWOperation(
		ctx,
		tb.Bus,
		&volume.BucketOpArgs{
			BucketId: bucketID,
			VolumeId: volID,
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer bhRel()

	putBlock := func(b block.Block) (*cid.BlockRef, error) {
		dat, err := b.MarshalBlock()
		if err != nil {
			return nil, err
		}
		ev, err := bk.PutBlock(dat, nil)
		if err != nil {
			return nil, err
		}
		return ev.GetBlockCommon().GetBlockRef(), nil
	}

	// store the root block.
	var rootBlock *cid.BlockRef
	if err := func() (err error) {
		rb := &Root{}
		ex := &Example{Msg: "hello world"}
		rb.ExamplePtr, err = putBlock(ex)
		if err != nil {
			return
		}
		rootBlock, err = putBlock(rb)
		return
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// br is the root block ref
	t.Logf("root block: %s", rootBlock.MarshalString())
	tr, cr := block.NewTransaction(bk, rootBlock, nil)
	data, found, err := cr.Fetch(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("data fetched: found (%v): %x", found, data)
	nri, err := cr.Unmarshal(
		ctx,
		func() block.Block {
			return &Root{}
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	nr := nri.(*Root)

	cptr, err := cr.FollowRef(ctx, 1, nr.GetExamplePtr())
	if err != nil {
		t.Fatal(err.Error())
	}

	exi, err := cptr.Unmarshal(
		ctx,
		func() block.Block {
			return &Example{}
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	ex := exi.(*Example)
	_ = tr
	t.Logf("got data from pointer block: %s", ex.GetMsg())
	if ex.GetMsg() != "hello world" {
		t.FailNow()
	}
	ex.Msg = "test data"
	cptr.SetBlock(ex)
	eves, err := tr.Write(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	for i, e := range eves {
		t.Logf("block %d written: %s", i, e.GetBlockCommon().GetBlockRef().MarshalString())
	}

	// test a new tx
	_, ncr := block.NewTransaction(
		bk,
		eves[len(eves)-1].GetBlockCommon().GetBlockRef(),
		nil,
	)
	ri, err := ncr.Unmarshal(ctx, func() block.Block { return &Root{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	nncr, err := ncr.FollowRef(ctx, 1, ri.(*Root).GetExamplePtr())
	if err != nil {
		t.Fatal(err.Error())
	}
	eei, err := nncr.Unmarshal(ctx, func() block.Block { return &Example{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	if eei.(*Example).GetMsg() != "test data" {
		t.FailNow()
	}
	t.Log("read written data correctly")
}
