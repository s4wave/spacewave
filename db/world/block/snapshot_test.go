package world_block_test

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// TestOpenSnapshotRecoversHistoricalWorld checks recovery without the original engine.
func TestOpenSnapshotRecoversHistoricalWorld(t *testing.T) {
	// Open a logger for the snapshot.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed whose block store holds the encrypted World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Encrypt every block with an inline transform the snapshot can carry.
	transform, err := block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      make([]byte, 32),
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Open the live World engine on an empty cursor.
	cursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, tb.Bus, le, tb.StepFactorySet, tb.BucketId, tb.Volume.GetID(), transform, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	engine, err := world_block.NewEngine(ctx, le, cursor, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	// Save the first committed root, then advance the live World.
	ws := world.NewEngineWorldState(engine, true)
	obj, err := ws.CreateObject(ctx, "saved", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Record the saved root, then create a later object.
	root := engine.GetRootRef()
	root.TransformConf = transform
	obj, err = ws.CreateObject(ctx, "later", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Close the live engine so only the saved root and block store remain.
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Open the saved root without the original engine.
	recovered, err := world_block.OpenSnapshot(ctx, le, tb.Volume, root)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	tx, err := recovered.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Require the saved object and reject the object created after the save.
	for key, want := range map[string]bool{"saved": true, "later": false} {
		obj, found, err := tx.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if found != want {
			t.Fatalf("historical object %s present=%t, want %t", key, found, want)
		}
	}

	// Require the recovered World to keep its changelog.
	entries, err := world_block.ReadChangeLogEntries(ctx, tx.AccessWorldState, world_block.ChangeLogReadOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("restored World lost its changelog")
	}
}
