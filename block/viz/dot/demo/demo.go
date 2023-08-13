package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_snappy "github.com/aperturerobotics/hydra/block/transform/snappy"
	"github.com/aperturerobotics/hydra/block/viz/dot"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := runDemo(); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

func runDemo() error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return err
	}

	vol := tb.Volume
	volID := vol.GetID()

	// store the bucket
	bucketID := "test-bucket-1"
	_, _, _, err = vol.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  bucketID,
		Rev: 1,
	})
	if err != nil {
		return err
	}
	le.Info(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_snappy.Config{},
	})
	if err != nil {
		return err
	}

	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		return err
	}

	tr := iavl.NewAVLTree(oc)
	atx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		err := atx.Set(ctx, key, key)
		if err != nil {
			return err
		}
	}
	if err := atx.Commit(ctx); err != nil {
		return err
	}

	atx, err = tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		return err
	}

	ops := []error{
		atx.Delete(ctx, []byte("key-0")),
		// atx.Delete([]byte("key-2")),
		atx.Delete(ctx, []byte("key-4")),
		/*
			atx.Delete([]byte("key-0")),
			atx.Delete([]byte("key-1")),
			atx.Delete([]byte("key-2")),
			atx.Delete([]byte("key-3")),
		*/
	}
	for _, op := range ops {
		if op != nil {
			return op
		}
	}

	if err := atx.Commit(ctx); err != nil {
		return err
	}

	btx, bcs := oc.BuildTransactionAtRef(nil, tr.GetRootNodeRef().GetRootRef())
	rn, err := block.UnmarshalBlock[*iavl.Node](ctx, bcs, iavl.NewNodeBlock)
	if err != nil {
		return err
	}

	err = dot.PlotToFile(ctx, "demo.dot", rn, btx, bcs, nil)
	if err != nil {
		return err
	}

	tr = iavl.NewAVLTree(oc)
	vtx, err := tr.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		return err
	}
	_, vExists, err := vtx.Get(ctx, []byte("key-3"))
	if err != nil {
		return err
	}
	if !vExists {
		return errors.New("key-3 does not exist")
	}
	vtx.Discard()
	return nil
}
