package main

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/block/viz/dot"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	"github.com/s4wave/spacewave/db/testbed"
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
	// Configure the context and logger for the tree visualization.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the demo tree.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return err
	}

	// Select the testbed volume used by the demo bucket.
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
		&transform_gzip.Config{},
	})
	if err != nil {
		return err
	}

	// Open an empty bucket cursor with the configured compression transform.
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

	// Populate and commit the demo AVL tree with five keys.
	tr := iavl.NewAVLTree(oc)
	atx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		return err
	}
	for i := range 5 {
		key := append([]byte("key-"), strconv.Itoa(i)...)
		err := atx.Set(ctx, key, key)
		if err != nil {
			return err
		}
	}
	if err := atx.Commit(ctx); err != nil {
		return err
	}

	// Open a new AVL transaction for removing the boundary keys.
	atx, err = tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Remove both boundary keys and check the deletion results.
	ops := []error{
		atx.Delete(ctx, []byte("key-0")),
		atx.Delete(ctx, []byte("key-4")),
	}
	for _, op := range ops {
		if op != nil {
			return op
		}
	}

	// Commit the tree after removing its boundary keys.
	if err := atx.Commit(ctx); err != nil {
		return err
	}

	// Read the committed AVL root for graph traversal.
	btx, bcs := oc.BuildTransactionAtRef(nil, tr.GetRootNodeRef().GetRootRef())
	rn, err := block.UnmarshalBlock[*iavl.Node](ctx, bcs, iavl.NewNodeBlock)
	if err != nil {
		return err
	}

	// Write the committed block graph to the demo DOT file.
	err = dot.PlotToFile(ctx, "demo.dot", rn, btx, bcs, nil)
	if err != nil {
		return err
	}

	// Reopen the tree and verify an interior key survived deletion.
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
