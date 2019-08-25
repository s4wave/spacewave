package block_mock

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/node"
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
	data, found, err := cr.Fetch()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("data fetched: found (%v): %x", found, data)
	nri, err := cr.Unmarshal(
		func() block.Block {
			return &Root{}
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	nr := nri.(*Root)

	cptr := cr.FollowRef(1, nr.GetExamplePtr())
	exi, err := cptr.Unmarshal(
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
	eves, cr, err := tr.Write()
	if err != nil {
		t.Fatal(err.Error())
	}
	for i, e := range eves {
		switch e.GetEventType() {
		case bucket_event.EventType_EventType_CUT_BLOCK:
			t.Logf(
				"block %d cut: %s",
				i, e.GetCutBlock().GetBlockCommon().GetBlockRef().MarshalString(),
			)
		case bucket_event.EventType_EventType_PUT_BLOCK:
			t.Logf(
				"block %d put: %s",
				i, e.GetPutBlock().GetBlockCommon().GetBlockRef().MarshalString(),
			)
		}
	}

	// test a new tx
	ncrRef := eves[len(eves)-1].GetPutBlock().GetBlockCommon().GetBlockRef()
	t.Logf(
		"ncr: %s",
		ncrRef.MarshalString(),
	)
	_, ncr := block.NewTransaction(
		bk,
		ncrRef,
		nil,
	)
	ri, err := ncr.Unmarshal(func() block.Block { return &Root{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	nncr := ncr.FollowRef(1, ri.(*Root).GetExamplePtr())
	eei, err := nncr.Unmarshal(func() block.Block { return &Example{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	if eei.(*Example).GetMsg() != "test data" {
		t.FailNow()
	}
	t.Log("read written data correctly")
}
