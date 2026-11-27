package wasivm_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// rootfsSource is the UnixFS object holding the root filesystem of a VM and
// the World it lives in.
type rootfsSource struct {
	// ws is the World holding the object.
	ws world.WorldState
	// key is the object key.
	key string
	// store is the CDN transport of the mounted CDN Space, or nil when the
	// object lives in the VM's own Space.
	store *cdn_bstore.CdnBlockStore
	// release closes the CDN Space mounted to read the object, if any.
	release func()
}

// resolveRootfs finds the root filesystem of the VM vmKey in ws: the
// wasivm/rootfs edge when set, else the root filesystem of the image linked by
// wasivm/image, which the run reads lazily.
func resolveRootfs(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	vmKey string,
) (*rootfsSource, error) {
	// Prefer the filesystem linked to the VM itself.
	key, ok, err := world.LookupGraphObjectKey(ctx, ws, vmKey, string(s4wave_vm.PredWasiVmRootfs))
	if err != nil {
		return nil, err
	}
	if ok {
		return &rootfsSource{ws: ws, key: key, release: func() {}}, nil
	}

	// Use the filesystem of the linked image.
	imageKey, ok, err := world.LookupGraphObjectKey(ctx, ws, vmKey, string(s4wave_vm.PredWasiVmImage))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("the VM has no root filesystem or image")
	}
	return resolveImageRootfs(ctx, le, b, ws, imageKey)
}

// resolveImageRootfs finds the root filesystem of the image imageKey in ws: its
// wasivmimage/rootfs edge, else that edge on the published image referenced by
// its cdn_source, whose CDN Space stays mounted until the source is released or
// ctx ends.
func resolveImageRootfs(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	imageKey string,
) (*rootfsSource, error) {
	// Prefer the filesystem linked to the image itself.
	key, ok, err := world.LookupGraphObjectKey(ctx, ws, imageKey, string(s4wave_vm.PredWasiVmImageRootfs))
	if err != nil {
		return nil, err
	}
	if ok {
		return &rootfsSource{ws: ws, key: key, release: func() {}}, nil
	}

	// Fall back to the published image the image reads from.
	src, err := readImageCdnSource(ctx, ws, imageKey)
	if err != nil {
		return nil, err
	}
	if src.GetSpaceId() == "" || src.GetObjectKey() == "" {
		return nil, errors.Errorf("image %s has no root filesystem", imageKey)
	}

	// Mount the CDN Space and find the published image's filesystem in it.
	baseURL := cdn.BaseURL()
	cdnWS, err := cdn_sharedobject.MountWorld(ctx, le, b, baseURL, src.GetSpaceId())
	if err != nil {
		return nil, errors.Wrapf(err, "mount the CDN Space %s from %s", src.GetSpaceId(), baseURL)
	}
	key, ok, err = world.LookupGraphObjectKey(ctx, cdnWS, src.GetObjectKey(), string(s4wave_vm.PredWasiVmImageRootfs))
	if err == nil && !ok {
		err = errors.Errorf("%s publishes no root filesystem for image %s/%s", baseURL, src.GetSpaceId(), src.GetObjectKey())
	}
	if err != nil {
		cdnWS.Release()
		return nil, err
	}
	return &rootfsSource{ws: cdnWS, key: key, store: cdnWS.Store, release: cdnWS.Release}, nil
}

// readImageCdnSource reads the cdn_source of the WasiVmImage at imageKey.
func readImageCdnSource(ctx context.Context, ws world.WorldState, imageKey string) (*s4wave_vm.VmImageCdnSource, error) {
	// Open the image object.
	objState, found, err := ws.GetObject(ctx, imageKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return nil, errors.Wrapf(err, "get image %s", imageKey)
	}
	if !found {
		return nil, errors.Errorf("image %s not found", imageKey)
	}

	// Copy the source out of the image block.
	var src *s4wave_vm.VmImageCdnSource
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		img, err := block.UnmarshalBlock[s4wave_vm.WasiVmImage](ctx, bcs)
		if err != nil {
			return err
		}
		src = img.GetCdnSource().CloneVT()
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "read image %s", imageKey)
	}
	return src, nil
}
