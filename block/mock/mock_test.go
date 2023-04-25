package block_mock

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
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
	_, _, bc, err := vol.ApplyBucketConfig(&bucket.Config{
		Id:  bucketID,
		Rev: 1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(volID)
	_ = bc

	bk, bhRel, err := bucket_lookup.StartBucketRWOperation(
		ctx,
		tb.Bus,
		&bucket.BucketOpArgs{
			BucketId: bucketID,
			VolumeId: volID,
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer bhRel()

	// store the root block.
	var rootBlock *block.BlockRef
	if err := func() (err error) {
		rb := &Root{}
		rb.ExampleSubBlock = &SubBlock{}
		ex := &Example{Msg: "hello world"}
		rb.ExampleSubBlock.ExamplePtr, _, err = block.PutBlock(bk, ex)
		if err != nil {
			return
		}
		rootBlock, _, err = block.PutBlock(bk, rb)
		return
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// br is the root block ref
	t.Logf("root block: %s", rootBlock.MarshalString())
	tr, cr := block.NewTransaction(bk, nil, rootBlock, nil)
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
	if nr.GetExampleSubBlock() == nil {
		t.Fail()
	}

	sbPtr := cr.FollowSubBlock(1)
	sbi, err := sbPtr.Unmarshal(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	sb := sbi.(*SubBlock)
	cptr := sbPtr.FollowRef(1, sb.GetExamplePtr())
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
	cptr.SetBlock(ex, true)
	blockRef, _, err := tr.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Logf(
		"block put: %s",
		blockRef.MarshalString(),
	)

	// test a new tx
	_, ncr := block.NewTransaction(
		bk,
		nil,
		blockRef,
		nil,
	)
	ri, err := ncr.Unmarshal(NewRootBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	sbcr := ncr.FollowSubBlock(1)
	nncr := sbcr.FollowRef(1, ri.(*Root).GetExampleSubBlock().GetExamplePtr())
	eei, err := nncr.Unmarshal(func() block.Block { return &Example{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	if eei.(*Example).GetMsg() != "test data" {
		t.FailNow()
	}
	t.Log("read written data correctly")

	// attempt to set a reference to a subblock from a new block
	_, cr = block.NewTransaction(bk, nil, blockRef, nil)
	ri, err = cr.Unmarshal(NewRootBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = ri
	sbcr = cr.FollowSubBlock(1)
	ncr = cr.Detach(false)
	ncr.SetBlock(NewSubBlockBlock, false)
	ncr.SetRef(1, sbcr)
	// expect the sub-block to be unlinked from the block.
	if sbcr.IsSubBlock() {
		t.Fail()
	}
}
