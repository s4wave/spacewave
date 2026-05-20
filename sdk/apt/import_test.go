package s4wave_apt

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

func TestImportDebPackageStoresBlockAndCreatesBuiltPackage(t *testing.T) {
	ctx, ws := setupAptWorldWithRepository(t)
	deb := testBusyboxDeb(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"

	aptPackage, debRef, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb)
	if err != nil {
		t.Fatalf("ImportDebPackage: %v", err)
	}
	if debRef.GetEmpty() {
		t.Fatal("deb ref is empty")
	}
	if got := aptPackage.GetState(); got != AptPackageState_AptPackageState_BUILT {
		t.Fatalf("state = %s, want BUILT", got.String())
	}
	if got := aptPackage.GetDebRef(); !got.EqualsRef(debRef) {
		t.Fatalf("package deb_ref = %s, want %s", got.MarshalString(), debRef.MarshalString())
	}
	if aptPackage.GetName() != "busybox" {
		t.Fatalf("name = %q, want busybox", aptPackage.GetName())
	}
	assertStoredDebBlock(t, ctx, ws, debRef, deb)

	stored := readAptBlock[*AptPackage](t, ctx, ws, packageKey, func() block.Block {
		return &AptPackage{}
	})
	if !stored.EqualVT(aptPackage) {
		t.Fatalf("stored package = %s, want %s", stored.String(), aptPackage.String())
	}
	assertObjectType(t, ctx, ws, packageKey, AptPackageTypeID)
	assertGraphEdge(t, ctx, ws, repositoryKey, PredAptRepoPackage.String(), packageKey)
}

func TestImportDebPackageCompletesExistingImportingPackage(t *testing.T) {
	ctx, ws := setupAptWorldWithRepository(t)
	deb := testBusyboxDeb(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"

	op := NewAddAptPackageOp(repositoryKey, packageKey, &AptPackage{
		State:        AptPackageState_AptPackageState_IMPORTING,
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
		Description:  "Tiny utilities",
	})
	if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("ApplyWorldOp(add importing package): %v", err)
	}

	aptPackage, debRef, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb)
	if err != nil {
		t.Fatalf("ImportDebPackage(existing): %v", err)
	}
	if got := aptPackage.GetState(); got != AptPackageState_AptPackageState_BUILT {
		t.Fatalf("state = %s, want BUILT", got.String())
	}
	assertStoredDebBlock(t, ctx, ws, debRef, deb)
	assertGraphEdge(t, ctx, ws, repositoryKey, PredAptRepoPackage.String(), packageKey)
}

func TestImportDebPackageRejectsExistingBuiltPackage(t *testing.T) {
	ctx, ws := setupAptWorldWithRepository(t)
	deb := testBusyboxDeb(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"

	if _, _, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb); err != nil {
		t.Fatalf("ImportDebPackage(create): %v", err)
	}
	if _, _, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb); !errors.Is(err, ErrInvalidAptPackageStateTransition) {
		t.Fatalf("ImportDebPackage(existing built) err = %v, want invalid transition", err)
	}
}

func TestImportDebPackageRejectsInvalidDebBeforeWorldMutation(t *testing.T) {
	ctx, ws := setupAptWorldWithRepository(t)
	packageKey := "apt/repos/stable/packages/busybox"
	if _, _, err := ImportDebPackage(ctx, ws, "apt/repos/stable", packageKey, []byte("deb")); !errors.Is(err, ErrInvalidDebPackage) {
		t.Fatalf("ImportDebPackage invalid deb err = %v, want invalid deb package", err)
	}
	_, found, err := ws.GetObject(ctx, packageKey)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if found {
		t.Fatal("invalid import created package object")
	}
}

func TestCompleteAptPackageImportRequiresDebRef(t *testing.T) {
	pkg, err := ParseDebPackage(testBusyboxDeb(t))
	if err != nil {
		t.Fatalf("ParseDebPackage: %v", err)
	}
	if _, err := CompleteAptPackageImport(pkg, nil); err == nil {
		t.Fatal("expected missing deb_ref error")
	}
}

func setupAptWorldWithRepository(t *testing.T) (context.Context, world.WorldState) {
	t.Helper()

	ctx, ws := setupAptWorld(t)
	createRepo := NewCreateAptRepositoryOp("apt/repos/stable", &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	})
	if _, _, err := ws.ApplyWorldOp(ctx, createRepo, ""); err != nil {
		t.Fatalf("ApplyWorldOp(create repository): %v", err)
	}
	return ctx, ws
}

func testBusyboxDeb(t *testing.T) []byte {
	t.Helper()

	control := "Package: busybox\n" +
		"Version: 1:1.36.1-7\n" +
		"Architecture: i386\n" +
		"Depends: libc6 (>= 2.34)\n" +
		"Description: Tiny utilities\n" +
		"\n"
	return buildDebFixture(t, "control.tar.gz", []byte(control))
}

func assertStoredDebBlock(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	debRef *block.BlockRef,
	want []byte,
) {
	t.Helper()

	cursor, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatalf("BuildStorageCursor: %v", err)
	}
	defer cursor.Release()

	got, found, err := cursor.GetBlock(ctx, debRef)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatalf("stored deb block %s not found", debRef.MarshalString())
	}
	if string(got) != string(want) {
		t.Fatal("stored deb block does not match input")
	}
}
