package traverse

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
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
	_, _, bc, err := vol.ApplyBucketConfig(ctx, &bucket.Config{
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
		rb := &block_mock.Root{}
		rb.ExampleSubBlock = &block_mock.SubBlock{}
		sb := rb.ExampleSubBlock
		ex := &block_mock.Example{Msg: "hello world"}
		sb.ExamplePtr, _, err = block.PutBlock(ctx, bk, ex)
		if err != nil {
			return
		}
		rootBlock, _, err = block.PutBlock(ctx, bk, rb)
		return
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// br is the root block ref
	t.Logf("root block: %s", rootBlock.MarshalString())
	_, cr := block.NewTransaction(bk, nil, rootBlock, nil)
	rii, err := cr.Unmarshal(ctx, func() block.Block { return &block_mock.Root{} })
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
		false,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}
