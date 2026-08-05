package s4wave_apt

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// DebPackageBlockWriter stores raw .deb package bytes.
type DebPackageBlockWriter interface {
	PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error)
}

// StoreDebPackageBlock stores .deb package data in the given block store.
func StoreDebPackageBlock(ctx context.Context, store DebPackageBlockWriter, deb []byte) (*block.BlockRef, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if len(deb) == 0 {
		return nil, errors.New("deb data is required")
	}
	ref, _, err := store.PutBlock(ctx, deb, &block.PutOpts{Sync: true})
	if err != nil {
		return nil, errors.Wrap(err, "store deb package")
	}
	return ref, nil
}

// CompleteAptPackageImport attaches a stored .deb ref and transitions to BUILT.
func CompleteAptPackageImport(pkg *AptPackage, debRef *block.BlockRef) (*AptPackage, error) {
	if pkg == nil {
		return nil, errors.New("apt_package is required")
	}
	if debRef.GetEmpty() {
		return nil, errors.New("deb_ref is required")
	}
	out := pkg.CloneVT()
	out.DebRef = debRef.Clone()
	if err := out.TransitionState(AptPackageState_AptPackageState_BUILT); err != nil {
		return nil, err
	}
	return out, nil
}

// ImportDebPackage stores package data and creates or completes an AptPackage.
func ImportDebPackage(
	ctx context.Context,
	ws world.WorldState,
	repositoryKey string,
	packageKey string,
	deb []byte,
) (*AptPackage, *block.BlockRef, error) {
	// Validate the world and package identifiers before importing data.
	if ws == nil {
		return nil, nil, errors.New("world state is required")
	}
	if repositoryKey == "" {
		return nil, nil, errors.New("repository_key is required")
	}
	if packageKey == "" {
		return nil, nil, errors.New("package_key is required")
	}
	if err := world_types.CheckObjectType(ctx, ws, repositoryKey, AptRepositoryTypeID); err != nil {
		return nil, nil, err
	}

	// Parse the Debian package and compute its checksums.
	parsed, err := ParseDebPackage(deb)
	if err != nil {
		return nil, nil, err
	}
	parsed.Checksums = AptPackageChecksums(deb)

	// Resolve the existing package target, if any.
	objectState, existing, err := lookupAptPackageImportTarget(ctx, ws, packageKey)
	if err != nil {
		return nil, nil, err
	}

	// Allocate storage and complete the imported package state.
	cursor, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build storage cursor")
	}
	defer cursor.Release()

	debRef, err := StoreDebPackageBlock(ctx, cursor, deb)
	if err != nil {
		return nil, nil, err
	}
	aptPackage, err := CompleteAptPackageImport(parsed, debRef)
	if err != nil {
		return nil, nil, err
	}

	// Create and initialize a package object when none exists.
	if objectState == nil {
		op := NewAddAptPackageOp(repositoryKey, packageKey, parsed)
		if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
			return nil, nil, err
		}
		objectState, err = world.MustGetObject(ctx, ws, packageKey)
		if err != nil {
			return nil, nil, err
		}
		if err := updateImportedAptPackage(ctx, objectState, parsed, aptPackage); err != nil {
			return nil, nil, err
		}
		return aptPackage, debRef, nil
	}

	// Update the existing package and restore its repository graph link.
	if err := updateImportedAptPackage(ctx, objectState, existing, aptPackage); err != nil {
		return nil, nil, err
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		repositoryKey,
		PredAptRepoPackage.String(),
		packageKey,
		"",
	)); err != nil {
		return nil, nil, err
	}
	return aptPackage, debRef, nil
}

func lookupAptPackageImportTarget(
	ctx context.Context,
	ws world.WorldState,
	packageKey string,
) (world.ObjectState, *AptPackage, error) {
	objectState, found, err := ws.GetObject(ctx, packageKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, nil
	}
	if err := world_types.CheckObjectType(ctx, ws, packageKey, AptPackageTypeID); err != nil {
		return nil, nil, err
	}
	existing, err := readAptPackageObject(ctx, objectState)
	if err != nil {
		return nil, nil, err
	}
	if existing.GetState() != AptPackageState_AptPackageState_IMPORTING {
		return nil, nil, errors.Wrapf(
			ErrInvalidAptPackageStateTransition,
			"%s -> %s",
			existing.GetState().String(),
			AptPackageState_AptPackageState_BUILT.String(),
		)
	}
	return objectState, existing, nil
}

func readAptPackageObject(ctx context.Context, objectState world.ObjectState) (*AptPackage, error) {
	var aptPackage *AptPackage
	_, _, err := world.AccessObjectState(ctx, objectState, false, func(bcs *block.Cursor) error {
		var unmarshalErr error
		aptPackage, unmarshalErr = block.UnmarshalBlock[*AptPackage](ctx, bcs, func() block.Block {
			return &AptPackage{}
		})
		return unmarshalErr
	})
	return aptPackage, err
}

func updateImportedAptPackage(
	ctx context.Context,
	objectState world.ObjectState,
	existing *AptPackage,
	aptPackage *AptPackage,
) error {
	next := aptPackage.CloneVT()
	next.State = existing.GetState()
	next.DebRef = nil
	completed, err := CompleteAptPackageImport(next, aptPackage.GetDebRef())
	if err != nil {
		return err
	}
	_, _, err = world.AccessObjectState(ctx, objectState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(completed, true)
		return nil
	})
	return err
}
