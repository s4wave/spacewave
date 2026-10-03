package space_migration_test

import (
	"context"
	"testing"

	space_migration "github.com/s4wave/spacewave/core/space/migration"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

func TestCanvasCombineIsTypedPerObjectKey(t *testing.T) {
	// Open source and destination Worlds for a Canvas merge collision.
	ctx := context.Background()
	source, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	destination, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Release()

	// Populate the Canvas collision and its source child object.
	setObject(t, ctx, source.WorldState, "child", s4wave_kv_world.KvStoreTypeID)
	setCanvasState(t, ctx, source.WorldState, "canvas-object", &s4wave_canvas.CanvasState{
		Nodes: map[string]*s4wave_canvas.CanvasNode{
			"node-1": {Id: "node-1", ObjectKey: "child"},
		},
	})
	setCanvasState(t, ctx, destination.WorldState, "canvas-object", &s4wave_canvas.CanvasState{
		Nodes: map[string]*s4wave_canvas.CanvasNode{
			"existing-node": {Id: "existing-node", ObjectKey: "child"},
		},
	})

	// Construct the built-in registry for Canvas migration.
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Plan a Canvas merge with the typed combine resolution.
	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID: "canvas-source", DestinationSpaceID: "canvas-destination",
		Source: source.WorldState, Destination: destination.WorldState,
		Operation:            space_migration.MigrationOperation_MIGRATION_OPERATION_MERGE,
		SelectedObjectKeys:   []string{"canvas-object"},
		CollisionResolutions: map[string]space_migration.MigrationConflictResolution{"canvas-object": space_migration.MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_COMBINE},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the combine preview provides a destination Canvas node identity.
	found := false
	for _, mapping := range preview.GetIdentityMappings() {
		if mapping.GetKind() == space_migration.MigrationReferenceKind_MIGRATION_REFERENCE_KIND_CANVAS_NODE && mapping.GetSource() == "node-1" {
			found = mapping.GetDestination() != ""
		}
	}
	if !found {
		t.Fatalf("Canvas node mapping missing from typed combine preview: %#v", preview.GetIdentityMappings())
	}
}

func TestCanvasRewriteRemapsNodeObjectKeyWhenCanvasRenamed(t *testing.T) {
	// Open a World testbed for rewriting a renamed Canvas.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Populate the Canvas and construct its migration registry.
	setCanvasState(t, ctx, tb.WorldState, "canvas-object", &s4wave_canvas.CanvasState{
		Nodes: map[string]*s4wave_canvas.CanvasNode{
			"node-1": {Id: "node-1", ObjectKey: "child"},
		},
	})
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Map the Canvas and child object keys to their renamed destinations.
	mapping := space_migration.NewIdentityMap()
	mapping.ObjectKeys["canvas-object"] = "canvas-object-renamed"
	mapping.ObjectKeys["child"] = "child-renamed"

	// Rewrite the Canvas through its registered handler.
	result, err := registry.Lookup(s4wave_canvas_world.CanvasTypeID).Rewrite(ctx, &space_migration.ObjectDescriptor{
		ObjectKey: "canvas-object", ObjectType: s4wave_canvas_world.CanvasTypeID, World: tb.WorldState,
	}, mapping)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the rewritten Canvas state.
	var rewritten s4wave_canvas.CanvasState
	if err := rewritten.UnmarshalVT(result.Payload); err != nil {
		t.Fatal(err)
	}

	// Verify the Canvas node retains its identity and maps its object key.
	node := rewritten.GetNodes()["node-1"]
	if node == nil || node.GetId() != "node-1" || node.GetObjectKey() != "child-renamed" {
		t.Fatalf("renamed Canvas node = %#v, want preserved ID and remapped object key", node)
	}
}

func TestCanvasRewriteRemapsNodeIDAndObjectKeyOnCombine(t *testing.T) {
	// Open a World testbed for combining Canvas node identities.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Populate the Canvas and construct its migration registry.
	setCanvasState(t, ctx, tb.WorldState, "canvas-object", &s4wave_canvas.CanvasState{
		Nodes: map[string]*s4wave_canvas.CanvasNode{
			"node-1": {Id: "node-1", ObjectKey: "child"},
		},
	})
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Map the Canvas node identity and its referenced child object key.
	mapping := space_migration.NewIdentityMap()
	mapping.ObjectKeys["canvas-object"] = "canvas-object"
	mapping.ObjectKeys["child"] = "child-renamed"
	mapping.CanvasNodes["node-1"] = "node-1-combined"

	// Rewrite the Canvas through its registered handler.
	result, err := registry.Lookup(s4wave_canvas_world.CanvasTypeID).Rewrite(ctx, &space_migration.ObjectDescriptor{
		ObjectKey: "canvas-object", ObjectType: s4wave_canvas_world.CanvasTypeID, World: tb.WorldState,
	}, mapping)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the rewritten Canvas state.
	var rewritten s4wave_canvas.CanvasState
	if err := rewritten.UnmarshalVT(result.Payload); err != nil {
		t.Fatal(err)
	}

	// Verify the Canvas node identity and object key are both remapped.
	node := rewritten.GetNodes()["node-1-combined"]
	if node == nil || node.GetId() != "node-1-combined" || node.GetObjectKey() != "child-renamed" {
		t.Fatalf("combined Canvas node = %#v, want remapped ID and object key", node)
	}
}
