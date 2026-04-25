package s4wave_vm_world

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// resolveV86Mount resolves a v86fs mount name to an FSHandle.
// Resolution order for each asset:
//  1. per-VM override edge on the VmV86 (v86/{asset}-override)
//  2. vmimage/{asset} edge on the VmImage linked via v86/image
//
// Supported mount names: ""/rootfs (rootfs), "kernel", "seabios", "vgabios",
// "wasm". The per-VM bios override (if present) covers both seabios and
// vgabios until a per-asset override is added.
func resolveV86Mount(ctx context.Context, ws world.WorldState, objectKey, name string) (*unixfs.FSHandle, error) {
	var overridePred, imagePred quad.IRI
	switch name {
	case "", "rootfs":
		overridePred = s4wave_vm.PredV86RootfsOverride
		imagePred = s4wave_vm.PredVmImageRootfs
	case "kernel":
		overridePred = s4wave_vm.PredV86KernelOverride
		imagePred = s4wave_vm.PredVmImageKernel
	case "seabios":
		overridePred = s4wave_vm.PredV86BiosOverride
		imagePred = s4wave_vm.PredVmImageBiosSeabios
	case "vgabios":
		overridePred = s4wave_vm.PredV86BiosOverride
		imagePred = s4wave_vm.PredVmImageBiosVgabios
	case "wasm":
		overridePred = s4wave_vm.PredV86WasmOverride
		imagePred = s4wave_vm.PredVmImageWasm
	default:
		return nil, unixfs_errors.ErrNotExist
	}

	targetKey, ok, err := lookupSingleEdge(ctx, ws, objectKey, string(overridePred))
	if err != nil {
		return nil, err
	}
	if ok {
		return openFSHandleForObject(ctx, ws, targetKey)
	}

	imageKey, ok, err := lookupSingleEdge(ctx, ws, objectKey, string(s4wave_vm.PredV86Image))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.Errorf("no v86/image edge on %s", objectKey)
	}
	assetKey, ok, err := lookupSingleEdge(ctx, ws, imageKey, string(imagePred))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.Errorf("vmimage %s has no %s edge", imageKey, imagePred)
	}
	return openFSHandleForObject(ctx, ws, assetKey)
}

// lookupSingleEdge returns the target object key of the first graph quad
// matching (subject, predicate, *). Reports (_, false, nil) when no edge is set.
func lookupSingleEdge(ctx context.Context, ws world.WorldState, subject, pred string) (string, bool, error) {
	gqs, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(subject, pred, "", ""),
		1,
	)
	if err != nil {
		return "", false, errors.Wrapf(err, "lookup graph edge %s on %s", pred, subject)
	}
	if len(gqs) == 0 {
		return "", false, nil
	}
	targetKey, err := world.GraphValueToKey(gqs[0].GetObj())
	if err != nil {
		return "", false, errors.Wrap(err, "parse target object key")
	}
	return targetKey, true, nil
}

// openFSHandleForObject opens a read-only FSHandle for a UnixFS world object.
func openFSHandleForObject(ctx context.Context, ws world.WorldState, objectKey string) (*unixfs.FSHandle, error) {
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "lookup fs type")
	}
	le := logrus.NewEntry(logrus.StandardLogger())
	fsCursor := unixfs_world.NewFSCursor(le, ws, objectKey, fsType, nil, false)
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, errors.Wrap(err, "create fs handle")
	}
	return fsh, nil
}
