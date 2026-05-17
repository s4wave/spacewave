package s4wave_apt

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

func TestAptWorldOpsCreateTypedEntitiesAndRepositoryEdges(t *testing.T) {
	ctx, ws := setupAptWorld(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"
	buildSpecKey := "apt/repos/stable/specs/busybox"

	createRepo := NewCreateAptRepositoryOp(repositoryKey, &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	})
	if _, _, err := ws.ApplyWorldOp(ctx, createRepo, ""); err != nil {
		t.Fatalf("ApplyWorldOp(create repository): %v", err)
	}

	debRef, err := block.BuildBlockRef([]byte("deb-payload"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef(deb): %v", err)
	}
	addPackage := NewAddAptPackageOp(repositoryKey, packageKey, &AptPackage{
		State:        AptPackageState_AptPackageState_BUILT,
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
		Description:  "Tiny utilities for small systems",
		DebRef:       debRef,
	})
	if _, _, err := ws.ApplyWorldOp(ctx, addPackage, ""); err != nil {
		t.Fatalf("ApplyWorldOp(add package): %v", err)
	}

	sourceRef, err := block.BuildBlockRef([]byte("busybox-source"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef(source): %v", err)
	}
	addBuildSpec := NewAddAptBuildSpecOp(repositoryKey, buildSpecKey, &AptBuildSpec{
		SourcePackage: "busybox",
		SourceRef:     &bucket.ObjectRef{RootRef: sourceRef},
		Architectures: []string{"i386"},
	})
	if _, _, err := ws.ApplyWorldOp(ctx, addBuildSpec, ""); err != nil {
		t.Fatalf("ApplyWorldOp(add build spec): %v", err)
	}

	assertObjectType(t, ctx, ws, repositoryKey, AptRepositoryTypeID)
	assertObjectType(t, ctx, ws, packageKey, AptPackageTypeID)
	assertObjectType(t, ctx, ws, buildSpecKey, AptBuildSpecTypeID)

	repository := readAptBlock[*AptRepository](t, ctx, ws, repositoryKey, func() block.Block {
		return &AptRepository{}
	})
	if repository.GetDistribution() != "stable" {
		t.Fatalf("repository distribution = %q, want stable", repository.GetDistribution())
	}
	aptPackage := readAptBlock[*AptPackage](t, ctx, ws, packageKey, func() block.Block {
		return &AptPackage{}
	})
	if aptPackage.GetName() != "busybox" {
		t.Fatalf("package name = %q, want busybox", aptPackage.GetName())
	}
	buildSpec := readAptBlock[*AptBuildSpec](t, ctx, ws, buildSpecKey, func() block.Block {
		return &AptBuildSpec{}
	})
	if buildSpec.GetSourcePackage() != "busybox" {
		t.Fatalf("build spec source package = %q, want busybox", buildSpec.GetSourcePackage())
	}

	assertGraphEdge(t, ctx, ws, repositoryKey, PredAptRepoPackage.String(), packageKey)
	assertGraphEdge(t, ctx, ws, repositoryKey, PredAptRepoBuildSpec.String(), buildSpecKey)
}

func TestLookupAptOp(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		id   string
	}{
		{name: "create repository", id: CreateAptRepositoryOpId},
		{name: "add package", id: AddAptPackageOpId},
		{name: "add build spec", id: AddAptBuildSpecOpId},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			op, err := LookupAptOp(ctx, test.id)
			if err != nil {
				t.Fatalf("LookupAptOp: %v", err)
			}
			if op == nil {
				t.Fatalf("LookupAptOp(%q) returned nil", test.id)
			}
			if op.GetOperationTypeId() != test.id {
				t.Fatalf("operation id = %q, want %q", op.GetOperationTypeId(), test.id)
			}
		})
	}

	op, err := LookupAptOp(ctx, "unknown/op")
	if err != nil {
		t.Fatalf("LookupAptOp(unknown): %v", err)
	}
	if op != nil {
		t.Fatalf("LookupAptOp(unknown) returned %T, want nil", op)
	}
}

func setupAptWorld(t *testing.T) (context.Context, world.WorldState) {
	t.Helper()

	ctx := t.Context()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	opController := world.NewLookupOpController("test-apt-ops", tb.EngineID, LookupAptOp)
	if _, err := tb.Bus.AddController(ctx, opController, nil); err != nil {
		t.Fatal(err)
	}

	return ctx, world.NewEngineWorldState(tb.Engine, true)
}

func assertObjectType(t *testing.T, ctx context.Context, ws world.WorldState, objectKey, typeID string) {
	t.Helper()

	got, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", objectKey, err)
	}
	if got != typeID {
		t.Fatalf("object %s type = %q, want %q", objectKey, got, typeID)
	}
}

func readAptBlock[T block.Block](
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	ctor func() block.Block,
) T {
	t.Helper()

	objectState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", objectKey, err)
	}
	if !found {
		t.Fatalf("object %s not found", objectKey)
	}

	var out T
	_, _, err = world.AccessObjectState(ctx, objectState, false, func(bcs *block.Cursor) error {
		var unmarshalErr error
		out, unmarshalErr = block.UnmarshalBlock[T](ctx, bcs, ctor)
		return unmarshalErr
	})
	if err != nil {
		t.Fatalf("AccessObjectState(%s): %v", objectKey, err)
	}
	return out
}

func assertGraphEdge(t *testing.T, ctx context.Context, ws world.WorldState, subjectKey, predicate, objectKey string) {
	t.Helper()

	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(subjectKey, predicate, objectKey, ""), 1)
	if err != nil {
		t.Fatalf("LookupGraphQuads(%s, %s, %s): %v", subjectKey, predicate, objectKey, err)
	}
	if len(quads) != 1 {
		t.Fatalf("expected graph edge %s --%s--> %s, got %d quads", subjectKey, predicate, objectKey, len(quads))
	}
}
