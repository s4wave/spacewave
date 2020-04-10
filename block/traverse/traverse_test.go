package traverse

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// TestVisit tests visiting a simple block graph.
func TestVisit(t *testing.T) {
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
		rb := &block_mock.Root{}
		rb.ExampleSubBlock = &block_mock.SubBlock{}
		sb := rb.ExampleSubBlock
		ex := &block_mock.Example{Msg: "hello world"}
		sb.ExamplePtr, err = putBlock(ex)
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
	_, cr := block.NewTransaction(bk, rootBlock, nil)
	rii, err := cr.Unmarshal(func() block.Block { return &block_mock.Root{} })
	if err != nil {
		t.Fatal(err.Error())
	}
	err = Visit(
		ctx,
		rii,
		cr,
		func(loc *Location) error {
			t.Logf(
				"Visit() called location depth %d refID %d ref %s",
				loc.Depth,
				loc.ParentRefID,
				loc.Cursor.GetRef().MarshalString(),
			)
			if ex, ok := loc.Block.(*block_mock.Example); ok {
				t.Logf("got data from pointer block: %s", ex.GetMsg())
				if ex.GetMsg() != "hello world" {
					t.FailNow()
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}
