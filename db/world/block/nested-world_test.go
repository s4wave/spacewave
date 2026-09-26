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
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)

	nestedRef, err := world_block.ImportSnapshot(ctx, tb.Engine, maps.All(map[string]block.Block{
		"inner": block_mock.NewExample("nested content"),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	// The receipt is an opaque app-owned block, separate from the standard root.
	receiptRef, err := world.AccessObject(ctx, tb.Engine.AccessWorldState, nil, func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample("source commit receipt"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

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

	err = world.ExecTransaction(ctx, tb.Engine, false, func(ctx context.Context, state world.WorldState) error {
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
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	const outerKey = "projection/replaced"

	publish := func(content string) *block.BlockRef {
		t.Helper()
		ref, err := world_block.ImportSnapshot(ctx, tb.Engine, maps.All(map[string]block.Block{
			"inner": block_mock.NewExample(content),
		}), nil)
		if err != nil {
			t.Fatal(err)
		}
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

	oldRoot := publish("before")
	newRoot := publish("after")
	if oldRoot.EqualsRef(newRoot) {
		t.Fatal("replacement produced the same root")
	}
	// World GC reconciles the publication before the volume sweeps blocks.
	gcTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer gcTx.Discard()
	if err := gcTx.(*world_block.EngineTx).GarbageCollect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := gcTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	collector := block_gc.NewCollector(tb.Volume.GetRefGraph(), tb.Volume, nil)
	if _, err := collector.Collect(ctx); err != nil {
		t.Fatal(err)
	}
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
