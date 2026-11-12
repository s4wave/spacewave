package s4wave_apt

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
)

func TestImportDebPackageStoresBlockAndCreatesBuiltPackage(t *testing.T) {
	// Prepare the repository, archive bytes, and package key.
	ctx, ws := setupAptWorldWithRepository(t)
	deb := testBusyboxDeb(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"

	// Import the Debian package into the World.
	aptPackage, debRef, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb)
	if err != nil {
		t.Fatalf("ImportDebPackage: %v", err)
	}

	// Verify the returned package metadata and stored archive reference.
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
	assertChecksumHex(t, aptPackage.GetChecksums(), "sha256", sha256Hex(deb))
	assertStoredDebBlock(t, ctx, ws, debRef, deb)

	// Verify the persisted package record, object type, and repository edge.
	stored := readAptBlock[AptPackage](t, ctx, ws, packageKey)
	if !stored.EqualVT(aptPackage) {
		t.Fatalf("stored package = %s, want %s", stored.String(), aptPackage.String())
	}
	assertObjectType(t, ctx, ws, packageKey, AptPackageTypeID)
	assertGraphEdge(t, ctx, ws, repositoryKey, PredAptRepoPackage.String(), packageKey)
}

func TestImportDebPackageCompletesExistingImportingPackage(t *testing.T) {
	// Prepare the repository and archive for an existing package import.
	ctx, ws := setupAptWorldWithRepository(t)
	deb := testBusyboxDeb(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"

	// Create an importing package object in the repository.
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

	// Complete the existing package import.
	aptPackage, debRef, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb)
	if err != nil {
		t.Fatalf("ImportDebPackage(existing): %v", err)
	}

	// Verify the completed package state, archive bytes, and repository edge.
	if got := aptPackage.GetState(); got != AptPackageState_AptPackageState_BUILT {
		t.Fatalf("state = %s, want BUILT", got.String())
	}
	assertStoredDebBlock(t, ctx, ws, debRef, deb)
	assertGraphEdge(t, ctx, ws, repositoryKey, PredAptRepoPackage.String(), packageKey)
}

func TestImportDebPackageRejectsExistingBuiltPackage(t *testing.T) {
	// Prepare a repository and package archive for duplicate import.
	ctx, ws := setupAptWorldWithRepository(t)
	deb := testBusyboxDeb(t)
	repositoryKey := "apt/repos/stable"
	packageKey := "apt/repos/stable/packages/busybox"

	// Verify an already built package rejects another import.
	if _, _, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb); err != nil {
		t.Fatalf("ImportDebPackage(create): %v", err)
	}
	if _, _, err := ImportDebPackage(ctx, ws, repositoryKey, packageKey, deb); !errors.Is(err, ErrInvalidAptPackageStateTransition) {
		t.Fatalf("ImportDebPackage(existing built) err = %v, want invalid transition", err)
	}
}

func TestImportDebPackageRejectsInvalidDebBeforeWorldMutation(t *testing.T) {
	// Reject invalid Debian bytes before checking the World remains unchanged.
	ctx, ws := setupAptWorldWithRepository(t)
	packageKey := "apt/repos/stable/packages/busybox"
	if _, _, err := ImportDebPackage(ctx, ws, "apt/repos/stable", packageKey, []byte("deb")); !errors.Is(err, ErrInvalidDebPackage) {
		t.Fatalf("ImportDebPackage invalid deb err = %v, want invalid deb package", err)
	}

	// Verify the failed import created no package object.
	objectState, found, err := ws.GetObject(ctx, packageKey)
	world.ReleaseObjectState(objectState)
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

// TestAptPackageRootHoldsDeb writes a deb and its package record the way
// ImportDebPackage does, the deb under a stage and the record through a block
// transaction, over a PendingStore. Once the stage is released, a sweep from
// the package root alone must keep the deb: the root's edges are all that
// hold it. The repository and build spec refs must be edges too.
func TestAptPackageRootHoldsDeb(t *testing.T) {
	ctx := t.Context()
	inner := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	ps := block.NewPendingStore(inner, 1<<20)
	deb := testBusyboxDeb(t)

	// Stage the deb as the import does.
	stage, releaseStage, err := ps.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	debRef, err := StoreDebPackageBlock(ctx, stage, deb)
	if err != nil {
		t.Fatal(err)
	}

	// Write the completed package record and release the stage.
	parsed, err := ParseDebPackage(deb)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := CompleteAptPackageImport(parsed, debRef)
	if err != nil {
		t.Fatal(err)
	}
	tx, bcs := block.NewTransaction(ps, nil, nil, nil)
	bcs.SetBlock(pkg, true)
	root, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	releaseStage()

	// A sweep from the package root keeps the deb.
	ps.Sweep([]*block.BlockRef{root}, ps.SweepMark())
	got, found, err := ps.GetBlock(ctx, debRef)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, deb) {
		t.Fatalf("deb after sweep from the package root: found=%v", found)
	}

	// The dormant refs are edges of their records.
	keyRef, err := block.BuildBlockRef([]byte("signing key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	indexRef, err := block.BuildBlockRef([]byte("index"), nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceRef, err := block.BuildBlockRef([]byte("source"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		blk  any
		want []*block.BlockRef
	}{
		{"repository", &AptRepository{SigningKeyRef: keyRef, IndexRef: &bucket.ObjectRef{RootRef: indexRef}}, []*block.BlockRef{keyRef, indexRef}},
		{"build spec", &AptBuildSpec{SourceRef: &bucket.ObjectRef{RootRef: sourceRef}}, []*block.BlockRef{sourceRef}},
	} {
		refs, err := block.ExtractBlockRefs(c.blk)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.EqualFunc(refs, c.want, (*block.BlockRef).EqualsRef) {
			t.Fatalf("%s edges = %v, want %v", c.name, refs, c.want)
		}
	}
}

func setupAptWorldWithRepository(t *testing.T) (context.Context, world.WorldState) {
	// Create the Apt repository in the prepared World.
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
	// Acquire a storage cursor for archive readback.
	t.Helper()
	cursor, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatalf("BuildStorageCursor: %v", err)
	}
	defer cursor.Release()

	// Verify the stored archive bytes match the input.
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
