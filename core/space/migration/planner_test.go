package space_migration_test

import (
	"context"
	"errors"
	"testing"

	space_migration "github.com/s4wave/spacewave/core/space/migration"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

func TestPlannerDeterminismClosureAndTypedMappings(t *testing.T) {
	// Initialize source and destination World testbeds.
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

	// Populate the source closure and construct the planner.
	setObject(t, ctx, source.WorldState, "root", s4wave_kv_world.KvStoreTypeID)
	setObject(t, ctx, source.WorldState, "child", s4wave_kv_world.KvStoreTypeID)
	setObject(t, ctx, source.WorldState, "secret", s4wave_kv_world.KvStoreTypeID)
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	planner := space_migration.NewPlanner(registry)
	input := &space_migration.PlannerInput{
		SourceSpaceID:      "source-space",
		DestinationSpaceID: "destination-space",
		Source:             source.WorldState,
		Destination:        destination.WorldState,
		SelectedObjectKeys: []string{"root"},
	}

	// Plan twice and compare deterministic output.
	first, err := planner.Plan(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := planner.Plan(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	// Verify closure totals and identity mappings.
	if first.Digest != second.Digest {
		t.Fatalf("digest changed between identical plans: %s != %s", first.Digest, second.Digest)
	}
	if first.Progress.GetObjectsPlanned() != 1 || first.Progress.GetLogicalBytes() == 0 {
		t.Fatalf("closure totals = (%d, %d), want one object with a scanned logical size", first.Progress.GetObjectsPlanned(), first.Progress.GetLogicalBytes())
	}
	if len(first.GetIdentityMappings()) < 2 {
		t.Fatalf("identity mapping count = %d, want object and block-store mappings", len(first.GetIdentityMappings()))
	}
}

func TestPlannerRefusesUnknownAndInsufficientCapacity(t *testing.T) {
	// Initialize testbeds for blocker and capacity cases.
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

	// Plan an unknown object and verify its blocker.
	setObject(t, ctx, source.WorldState, "unknown", "plugin/unknown")
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	planner := space_migration.NewPlanner(registry)
	preview, err := planner.Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID:      "source-space",
		DestinationSpaceID: "destination-space",
		Source:             source.WorldState,
		Destination:        destination.WorldState,
	})
	if !errors.Is(err, space_migration.ErrPlanBlocked) {
		t.Fatalf("unknown type error = %v, want ErrPlanBlocked", err)
	}
	foundUnknown := false
	for _, blocker := range preview.GetBlockers() {
		if blocker.GetObjectType() == "plugin/unknown" && len(blocker.GetObjectKeys()) != 0 && blocker.GetObjectKeys()[0] == "unknown" {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatalf("unknown type blocker = %#v", preview.GetBlockers())
	}

	// Plan a known object against insufficient destination capacity.
	setObject(t, ctx, source.WorldState, "known", s4wave_kv_world.KvStoreTypeID)
	preview, err = planner.Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID:       "source-space",
		DestinationSpaceID:  "destination-space",
		Source:              source.WorldState,
		Destination:         destination.WorldState,
		SelectedObjectKeys:  []string{"known"},
		CapacityKnown:       true,
		DestinationCapacity: 9,
	})
	if !errors.Is(err, space_migration.ErrCapacityInsufficient) {
		t.Fatalf("capacity error = %v, want ErrCapacityInsufficient", err)
	}
	if preview.GetResult().GetCode() != "preview-blocked" {
		t.Fatalf("capacity result = %#v", preview.GetResult())
	}
}

func TestPlannerRejectsStaleWorld(t *testing.T) {
	// Initialize worlds and a baseline preview.
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
	setObject(t, ctx, source.WorldState, "known", s4wave_kv_world.KvStoreTypeID)
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	planner := space_migration.NewPlanner(registry)
	input := &space_migration.PlannerInput{
		SourceSpaceID: "source-space", DestinationSpaceID: "destination-space",
		Source: source.WorldState, Destination: destination.WorldState, SelectedObjectKeys: []string{"known"},
	}
	preview, err := planner.Plan(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the source after planning and reject the stale preview.
	setObject(t, ctx, source.WorldState, "later", s4wave_kv_world.KvStoreTypeID)
	if err := planner.VerifyFresh(ctx, input, preview); !errors.Is(err, space_migration.ErrStalePlan) {
		t.Fatalf("stale verification error = %v, want ErrStalePlan", err)
	}
}

func TestPlannerUsesDeterministicCollisionSuggestion(t *testing.T) {
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
	setObject(t, ctx, source.WorldState, "dir/file", s4wave_kv_world.KvStoreTypeID)
	setObject(t, ctx, destination.WorldState, "dir/file", s4wave_canvas_world.CanvasTypeID)
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID: "source-space", DestinationSpaceID: "destination-space",
		Source: source.WorldState, Destination: destination.WorldState,
		Operation:            space_migration.MigrationOperation_MIGRATION_OPERATION_MERGE,
		SelectedObjectKeys:   []string{"dir/file"},
		CollisionResolutions: map[string]space_migration.MigrationConflictResolution{"dir/file": space_migration.MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_RENAME},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.GetConflicts()) != 1 || preview.GetConflicts()[0].GetSuggestedKey() != "dir/file~ce-space" {
		t.Fatalf("collision conflict = %#v", preview.GetConflicts())
	}
	if preview.GetIdentityMappings()[0].GetDestination() != "dir/file~ce-space" {
		t.Fatalf("collision mapping = %#v", preview.GetIdentityMappings())
	}
}

func TestPlannerRemapsDescendantClosureKeys(t *testing.T) {
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
	setObject(t, ctx, source.WorldState, "dir", s4wave_kv_world.KvStoreTypeID)
	setObject(t, ctx, source.WorldState, "dir/child", s4wave_kv_world.KvStoreTypeID)
	setObject(t, ctx, destination.WorldState, "dir", s4wave_canvas_world.CanvasTypeID)
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID: "source-space", DestinationSpaceID: "destination-space",
		Source: source.WorldState, Destination: destination.WorldState,
		Operation:            space_migration.MigrationOperation_MIGRATION_OPERATION_MERGE,
		SelectedObjectKeys:   []string{"dir"},
		CollisionResolutions: map[string]space_migration.MigrationConflictResolution{"dir": space_migration.MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_RENAME},
	})
	if err != nil {
		t.Fatal(err)
	}
	mappings := make(map[string]string)
	for _, mapping := range preview.GetIdentityMappings() {
		if mapping.GetKind() == space_migration.MigrationReferenceKind_MIGRATION_REFERENCE_KIND_OBJECT_KEY {
			mappings[mapping.GetSource()] = mapping.GetDestination()
		}
	}
	if mappings["dir"] != "dir~ce-space" {
		t.Fatalf("descendant mappings = %#v", mappings)
	}
}

func setObject(t *testing.T, ctx context.Context, ws world.WorldState, key, typeID string) {
	t.Helper()
	var root *bucket.ObjectRef
	err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		root = cursor.GetRef()
		tx, blocks := cursor.BuildTransactionAtRef(nil, nil)
		switch typeID {
		case s4wave_kv_world.KvStoreTypeID:
			blocks.SetBlock(kvtx_block.NewKeyValueStore(0), true)
		case s4wave_canvas_world.CanvasTypeID:
			blocks.SetBlock(s4wave_canvas.NewCanvasStorage(), true)
		default:
			blocks.SetBlock(block_mock.NewExampleBlock(), true)
		}
		var err error
		root.RootRef, _, err = tx.Write(ctx, true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.CreateObject(ctx, key, root); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
		t.Fatal(err)
	}
}

func setCanvasState(t *testing.T, ctx context.Context, ws world.WorldState, key string, state *s4wave_canvas.CanvasState) {
	t.Helper()
	_, _, err := world.CreateWorldObject(ctx, ws, key, func(blocks *block.Cursor) error {
		return s4wave_canvas.WriteCanvasState(ctx, blocks, nil, state)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, key, s4wave_canvas_world.CanvasTypeID); err != nil {
		t.Fatal(err)
	}
}

func setObjectBlock(t *testing.T, ctx context.Context, ws world.WorldState, key, typeID string, payload block.Block) {
	t.Helper()
	var root *bucket.ObjectRef
	err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		root = cursor.GetRef()
		tx, blocks := cursor.BuildTransactionAtRef(nil, nil)
		blocks.SetBlock(payload, true)
		var err error
		root.RootRef, _, err = tx.Write(ctx, true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.CreateObject(ctx, key, root); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
		t.Fatal(err)
	}
}
