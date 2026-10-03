package world_block_test

import (
	"context"
	"maps"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// TestNestedWorldBucketBoundary rejects a foreign root instead of silently
// redirecting it into the enclosing bucket.
func TestNestedWorldBucketBoundary(t *testing.T) {
	// Build a nested World and require the local bucket id.
	worldRef := &bucket.ObjectRef{
		BucketId:      "home",
		TransformConf: &block_transform.Config{Steps: []*block_transform.StepConfig{{Id: "test"}}},
	}
	payloadRef := &bucket.ObjectRef{BucketId: "home"}
	for _, refs := range []struct {
		name    string
		world   *bucket.ObjectRef
		payload *bucket.ObjectRef
	}{
		{name: "foreign world", world: &bucket.ObjectRef{BucketId: "foreign"}},
		{name: "foreign payload", world: worldRef, payload: &bucket.ObjectRef{BucketId: "foreign"}},
	} {
		t.Run(refs.name, func(t *testing.T) {
			if nested, err := world_block.NewNestedWorld("home", refs.world, refs.payload); err == nil || nested != nil {
				t.Fatalf("foreign root accepted: nested %v, error %v", nested, err)
			}
		})
	}
	nested, err := world_block.NewNestedWorld("home", worldRef, payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	local := worldRef.Clone()
	local.BucketId = ""
	if !nested.GetWorldRef().EqualVT(local) || worldRef.GetBucketId() != "home" {
		t.Fatal("local reference lost its transform or mutated the source")
	}
}

// TestNestedWorldPublication verifies that a typed outer object retains an
// immutable, independently readable World snapshot through a block roundtrip.
func TestNestedWorldPublication(t *testing.T) {
	// Open a testbed World.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)

	// Import the nested snapshot.
	nestedRef, err := world_block.ImportSnapshot(ctx, engineStage(t, tb.Engine), maps.All(map[string]block.Block{
		"inner": block_mock.NewExample("nested content"),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	// The receipt is an opaque app-owned block, separate from the standard root,
	// staged until the publication adopts it.
	receiptRef, err := world.AccessObject(ctx, engineStage(t, tb.Engine).AccessWorldState, nil, func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample("source commit receipt"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish the nested World under the outer key.
	const outerKey = "projection/example"
	err = world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, state world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, outerKey, true, func(cursor *block.Cursor) error {
			nested, err := world_block.NewNestedWorld(tb.EngineBucketID, nestedRef, receiptRef)
			if err != nil {
				return err
			}
			cursor.SetBlock(nested, true)
			return nil
		})
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, state, outerKey, "example/projection")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read the published nested World back.
	err = world.ExecTransaction(ctx, tb.Engine, false, func(ctx context.Context, state world.WorldState) error {
		// Require the published type.
		typeID, err := world_types.GetObjectType(ctx, state, outerKey)
		if err != nil {
			return err
		}
		if typeID != "example/projection" {
			t.Fatalf("outer type = %q", typeID)
		}
		published, err := world.LookupObjectBody[*world_block.NestedWorld](ctx, state, outerKey, world_block.NewNestedWorldBlock)
		if err != nil {
			return err
		}

		// Require the published refs to drop the outer bucket id.
		localReceipt := receiptRef.Clone()
		localReceipt.BucketId = ""
		if !published.GetPayloadRef().EqualVT(localReceipt) {
			t.Fatalf("published receipt ref = %v, want %v", published.GetPayloadRef(), localReceipt)
		}
		localWorld := nestedRef.Clone()
		localWorld.BucketId = ""
		if !published.GetWorldRef().EqualVT(localWorld) {
			t.Fatalf("published World ref = %v, want %v", published.GetWorldRef(), localWorld)
		}
		if err := tb.Engine.AccessWorldState(ctx, published.GetPayloadRef(), func(cursor *bucket_lookup.Cursor) error {
			// Require the payload block to hold the source receipt.
			_, bcs := cursor.BuildTransaction(nil)
			body, err := block.UnmarshalBlock[*block_mock.Example](ctx, bcs, block_mock.NewExampleBlock)
			if err != nil {
				return err
			}
			if body.GetMsg() != "source commit receipt" {
				t.Fatalf("receipt = %q", body.GetMsg())
			}
			return nil
		}); err != nil {
			return err
		}
		return tb.Engine.AccessWorldState(ctx, published.GetWorldRef(), func(cursor *bucket_lookup.Cursor) error {
			// Open the nested World and require its inner object.
			nested, err := world_block.BuildWorldStateFromCursor(ctx, tb.Logger, false, cursor, tb.Engine, nil, false)
			if err != nil {
				return err
			}
			defer nested.Discard()
			body, err := world.LookupObjectBody[*block_mock.Example](ctx, nested, "inner", block_mock.NewExampleBlock)
			if err != nil {
				return err
			}
			if body.GetMsg() != "nested content" {
				t.Fatalf("inner body = %q", body.GetMsg())
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNestedWorldReplacementGC verifies that replacing a published local root
// drops the old block graph while retaining the new snapshot.
func TestNestedWorldReplacementGC(t *testing.T) {
	// Build the World testbed.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	const outerKey = "projection/replaced"

	// publish imports a snapshot and stores it as the outer object's nested World.
	publish := func(content string) *block.BlockRef {
		// Import the snapshot through a stage held until the outer object
		// adopts it.
		t.Helper()
		stage, err := tb.Engine.StageWorldState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer stage.Release()
		ref, err := world_block.ImportSnapshot(ctx, stage, maps.All(map[string]block.Block{
			"inner": block_mock.NewExample(content),
		}), nil)
		if err != nil {
			t.Fatal(err)
		}

		// Point the outer object at the snapshot.
		if err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, state world.WorldState) error {
			_, _, err := world.AccessWorldObject(ctx, state, outerKey, true, func(cursor *block.Cursor) error {
				nested, err := world_block.NewNestedWorld(tb.EngineBucketID, ref, nil)
				if err != nil {
					return err
				}
				cursor.SetBlock(nested, true)
				return nil
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return ref.GetRootRef()
	}

	// Publish a snapshot, then replace it.
	oldRoot := publish("before")
	newRoot := publish("after")
	if oldRoot.EqualsRef(newRoot) {
		t.Fatal("replacement produced the same root")
	}

	// The volume collects the replaced root without World-local bookkeeping.
	collector := block_gc.NewCollector(tb.Volume.GetRefGraph(), tb.Volume, nil)
	if _, err := collector.Collect(ctx); err != nil {
		t.Fatal(err)
	}

	// Check the old root is gone and the new root is retained.
	oldExists, err := tb.Volume.GetBlockExists(ctx, oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	if oldExists {
		t.Fatal("replaced nested World root remains retained after collection")
	}
	newExists, err := tb.Volume.GetBlockExists(ctx, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !newExists {
		t.Fatal("published nested World root was collected")
	}
	if err := world.ExecTransaction(ctx, tb.Engine, false, func(ctx context.Context, state world.WorldState) error {
		outer, err := world.LookupObjectBody[*world_block.NestedWorld](ctx, state, outerKey, world_block.NewNestedWorldBlock)
		if err != nil {
			return err
		}
		return tb.Engine.AccessWorldState(ctx, outer.GetWorldRef(), func(cursor *bucket_lookup.Cursor) error {
			// Open the updated nested World and require the new inner object.
			nested, err := world_block.BuildWorldStateFromCursor(ctx, tb.Logger, false, cursor, tb.Engine, nil, false)
			if err != nil {
				return err
			}
			defer nested.Discard()
			body, err := world.LookupObjectBody[*block_mock.Example](ctx, nested, "inner", block_mock.NewExampleBlock)
			if err != nil {
				return err
			}
			if body.GetMsg() != "after" {
				t.Fatalf("nested body = %q, want after", body.GetMsg())
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
}
