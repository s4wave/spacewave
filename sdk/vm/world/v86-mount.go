package s4wave_vm_world

import (
	"context"
	"sync"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// v86MountResolver resolves the v86fs mounts of one VmV86 object. It mounts
// the CDN Space of a linked V86Image on first use and keeps that World open
// until release, so asset handles read the CDN lazily for the VM's lifetime.
type v86MountResolver struct {
	le        *logrus.Entry
	b         bus.Bus
	ws        world.WorldState
	objectKey string

	// ctx scopes the CDN Worlds and is canceled by release.
	ctx    context.Context
	cancel context.CancelFunc

	mtx       sync.Mutex
	cdnWorlds map[string]world.WorldState
	releases  []func()
}

// newV86MountResolver constructs a resolver whose CDN Worlds outlive ctx's
// cancellation until release is called.
func newV86MountResolver(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	objectKey string,
) *v86MountResolver {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &v86MountResolver{
		le:        le,
		b:         b,
		ws:        ws,
		objectKey: objectKey,
		ctx:       ctx,
		cancel:    cancel,
		cdnWorlds: make(map[string]world.WorldState),
	}
}

// resolve resolves a v86fs mount name to an FSHandle.
// Resolution order for each asset:
//  1. per-VM override edge on the VmV86 (v86/{asset}-override)
//  2. v86image/{asset} edge on the V86Image linked via v86/image
//  3. v86image/{asset} edge on the image referenced by the V86Image cdn_source
//
// Supported mount names: ""/rootfs (rootfs), "kernel", "seabios", "vgabios",
// "wasm". The per-VM bios override (if present) covers both seabios and
// vgabios until a per-asset override is added.
func (r *v86MountResolver) resolve(ctx context.Context, name string) (*unixfs.FSHandle, error) {
	// Select the VM override and image asset predicates for the requested mount.
	overridePred, imagePred, ok := v86AssetPreds(name)
	if !ok {
		return nil, unixfs_errors.ErrNotExist
	}

	// Resolve the VM override before consulting the linked image.
	targetKey, ok, err := world.LookupGraphObjectKey(ctx, r.ws, r.objectKey, string(overridePred))
	if err != nil {
		return nil, err
	}
	if ok {
		return openFSHandleForObject(ctx, r.le, r.ws, targetKey)
	}

	// Open the requested asset of the linked image.
	imageKey, ok, err := world.LookupGraphObjectKey(ctx, r.ws, r.objectKey, string(s4wave_vm.PredV86Image))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.Errorf("no v86/image edge on %s", r.objectKey)
	}
	return r.openImageAsset(ctx, imageKey, imagePred)
}

// v86AssetPreds returns the VM override and image predicates of the asset
// mounted as name, or false for an unknown name.
func v86AssetPreds(name string) (overridePred, imagePred quad.IRI, ok bool) {
	switch name {
	case "", "rootfs":
		return s4wave_vm.PredV86RootfsOverride, s4wave_vm.PredV86ImageRootfs, true
	case "kernel":
		return s4wave_vm.PredV86KernelOverride, s4wave_vm.PredV86ImageKernel, true
	case "seabios":
		return s4wave_vm.PredV86BiosOverride, s4wave_vm.PredV86ImageBiosSeabios, true
	case "vgabios":
		return s4wave_vm.PredV86BiosOverride, s4wave_vm.PredV86ImageBiosVgabios, true
	case "wasm":
		return s4wave_vm.PredV86WasmOverride, s4wave_vm.PredV86ImageWasm, true
	default:
		return "", "", false
	}
}

// openImageAsset opens the imagePred asset of the V86Image imageKey: the
// image's own edge, else the edge on the published image its cdn_source
// references.
func (r *v86MountResolver) openImageAsset(ctx context.Context, imageKey string, imagePred quad.IRI) (*unixfs.FSHandle, error) {
	// Prefer the asset linked to the image itself.
	assetKey, ok, err := world.LookupGraphObjectKey(ctx, r.ws, imageKey, string(imagePred))
	if err != nil {
		return nil, err
	}
	if ok {
		return openFSHandleForObject(ctx, r.le, r.ws, assetKey)
	}

	// Fall back to the published image the image reads from.
	src, err := readV86ImageCdnSource(ctx, r.ws, imageKey)
	if err != nil {
		return nil, err
	}
	if src.GetSpaceId() == "" || src.GetObjectKey() == "" {
		return nil, errors.Errorf("v86 image %s has no %s edge", imageKey, imagePred)
	}

	// Resolve the asset edge on the published image in its CDN World.
	cdnWS, err := r.mountCdnWorld(src.GetSpaceId())
	if err != nil {
		return nil, err
	}
	assetKey, ok, err = world.LookupGraphObjectKey(ctx, cdnWS, src.GetObjectKey(), string(imagePred))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.Errorf(
			"cdn v86 image %s/%s has no %s edge",
			src.GetSpaceId(), src.GetObjectKey(), imagePred,
		)
	}
	return openFSHandleForObject(ctx, r.le, cdnWS, assetKey)
}

// mountCdnWorld returns the World of the CDN Space spaceID, mounting it on
// first use.
func (r *v86MountResolver) mountCdnWorld(spaceID string) (world.WorldState, error) {
	// Serialize mounts so each CDN Space is mounted once.
	r.mtx.Lock()
	defer r.mtx.Unlock()

	// Reuse a mounted World and refuse new mounts after release.
	if ws, ok := r.cdnWorlds[spaceID]; ok {
		return ws, nil
	}
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	// Mount the CDN World and hold it until release.
	mounted, err := cdn_sharedobject.MountWorld(r.ctx, r.le, r.b, cdn.BaseURL(), spaceID)
	if err != nil {
		return nil, errors.Wrapf(err, "mount cdn space %s", spaceID)
	}
	r.cdnWorlds[spaceID] = mounted
	r.releases = append(r.releases, mounted.Release)
	return mounted, nil
}

// release closes every mounted CDN World.
func (r *v86MountResolver) release() {
	// Stop new mounts and cancel reads in flight.
	r.cancel()

	// Take the mounted Worlds out of the resolver.
	r.mtx.Lock()
	releases := r.releases
	r.releases = nil
	clear(r.cdnWorlds)
	r.mtx.Unlock()

	// Close each World and its CDN transport.
	for _, rel := range releases {
		rel()
	}
}

// readV86ImageCdnSource reads the cdn_source of the V86Image at imageKey.
func readV86ImageCdnSource(ctx context.Context, ws world.WorldState, imageKey string) (*s4wave_vm.VmImageCdnSource, error) {
	// Open the image object.
	objState, found, err := ws.GetObject(ctx, imageKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return nil, errors.Wrapf(err, "get v86 image %s", imageKey)
	}
	if !found {
		return nil, errors.Errorf("v86 image %s not found", imageKey)
	}

	// Copy the source out of the image block.
	var src *s4wave_vm.VmImageCdnSource
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		img, err := block.UnmarshalBlock[s4wave_vm.V86Image](ctx, bcs)
		if err != nil {
			return err
		}
		src = img.GetCdnSource().CloneVT()
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "read v86 image %s", imageKey)
	}
	return src, nil
}

// openFSHandleForObject opens a read-only FSHandle for a UnixFS world object.
func openFSHandleForObject(ctx context.Context, le *logrus.Entry, ws world.WorldState, objectKey string) (*unixfs.FSHandle, error) {
	// Open the UnixFS object cursor and transfer it to a filesystem handle.
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "lookup fs type")
	}
	fsCursor := unixfs_world.NewFSCursor(le, ws, objectKey, fsType, nil, false)
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, errors.Wrap(err, "create fs handle")
	}
	return fsh, nil
}
