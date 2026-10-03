package dot

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestPlotToFileWritesRequestedPath tests that PlotToFile writes the plot to
// the requested output path.
func TestPlotToFileWritesRequestedPath(t *testing.T) {
	// Prepare a context and logger for the block graph testbed.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the storage testbed and release it after plotting.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Configure the test bucket in the testbed's volume.
	volID := tb.Volume.GetID()
	if _, _, _, err = tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  "test-bucket-1",
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Configure gzip compression for the graph's stored blocks.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor with the compressed block configuration.
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
		t.Fatal(err.Error())
	}

	// Populate and commit an AVL tree for the block graph plot.
	tr := iavl.NewAVLTree(oc)
	atx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	for i := range 5 {
		key := append([]byte("key-"), strconv.Itoa(i)...)
		if err := atx.Set(ctx, key, key); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := atx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Load the committed tree root into a block graph transaction.
	btx, bcs := oc.BuildTransactionAtRef(nil, tr.GetRootNodeRef().GetRootRef())
	rn, err := block.UnmarshalBlock[*iavl.Node](ctx, bcs, iavl.NewNodeBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Plot the committed tree at a temporary output path.
	outPath := filepath.Join(t.TempDir(), "plot-out.dot")
	if err := PlotToFile(ctx, outPath, rn, btx, bcs, nil); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the DOT plot exists at the requested output path.
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected plot at %s: %v", outPath, err)
	}
}
