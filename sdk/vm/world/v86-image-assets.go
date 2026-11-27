package s4wave_vm_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// V86ImageAssets opens the assets of one V86Image: each from the image's own
// edge, else from the published image its cdn_source references, whose CDN
// Space stays mounted until Release so the handles read it lazily.
type V86ImageAssets struct {
	// r resolves the assets and holds the CDN Worlds.
	r *v86MountResolver
	// imageKey is the V86Image object key.
	imageKey string
}

// NewV86ImageAssets opens the assets of the V86Image imageKey in ws. The CDN
// Worlds it mounts outlive ctx until Release.
func NewV86ImageAssets(ctx context.Context, le *logrus.Entry, b bus.Bus, ws world.WorldState, imageKey string) *V86ImageAssets {
	return &V86ImageAssets{r: newV86MountResolver(ctx, le, b, ws, ""), imageKey: imageKey}
}

// Open opens the asset name: "rootfs", "kernel", "seabios", "vgabios" or
// "wasm". The caller releases the handle before Release.
func (a *V86ImageAssets) Open(ctx context.Context, name string) (*unixfs.FSHandle, error) {
	_, imagePred, ok := v86AssetPreds(name)
	if !ok {
		return nil, unixfs_errors.ErrNotExist
	}
	return a.r.openImageAsset(ctx, a.imageKey, imagePred)
}

// Release closes the CDN Worlds the assets read from.
func (a *V86ImageAssets) Release() {
	a.r.release()
}
