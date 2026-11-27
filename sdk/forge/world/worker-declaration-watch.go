//go:build !tinygo && !js

package s4wave_forge_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// workerDeclarationWatch follows one World's Forge Worker declaration bound to
// the native host Root.
type workerDeclarationWatch struct {
	// ctx ends pending waits when the watch closes.
	ctx context.Context
	// source supplies the current and changed declarations.
	source plugin_host_root.ForgeWorkerSource
	// engineID is the World engine ID whose declaration the watch follows.
	engineID string
	// last is the revision of the most recently received declaration.
	last uint64
	// release cancels ctx and releases the Root reference.
	release func()
}

// openWorkerDeclarationWatch looks up the native host Root, which the Space
// runtime bridges into its generation, and waits for the daemon to bind its
// Forge Worker source. The watch follows the declaration of World engineID.
func openWorkerDeclarationWatch(ctx context.Context, b bus.Bus, engineID string) (*workerDeclarationWatch, error) {
	// Find the Root that owns this process's daemon declaration.
	ctx, cancel := context.WithCancel(ctx)
	root, _, rootRef, err := plugin_host_root.ExLookupRootByPlatform(
		ctx, b, false, (&bldr_platform.NativePlatform{}).GetPlatformID(), nil,
	)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "look up native host root")
	}

	// Wait for the daemon to bind its source.
	source, err := root.WaitForgeWorkerSource(ctx)
	if err != nil {
		rootRef.Release()
		cancel()
		return nil, err
	}
	return &workerDeclarationWatch{ctx: ctx, source: source, engineID: engineID, release: func() {
		cancel()
		rootRef.Release()
	}}, nil
}

// Recv waits for the first or next declaration and its enrolled Device
// identity. The declaration is nil while the Device declares no Worker.
func (w *workerDeclarationWatch) Recv() (*s4wave_device.ForgeWorkerDeclaration, string, error) {
	// Wait for a revision other than the last one received.
	data, deviceKey, revision, err := w.source.WaitForgeWorker(w.ctx, w.engineID, w.last)
	if err != nil {
		return nil, "", err
	}
	w.last = revision

	// Decode the declaration data, treating a declaration with no Worker as
	// none.
	declaration := &s4wave_device.ForgeWorkerDeclaration{}
	if err := declaration.UnmarshalVT(data); err != nil {
		return nil, "", errors.Wrap(err, "decode forge worker declaration")
	}
	if declaration.GetWorkerObjectKey() == "" {
		return nil, deviceKey, nil
	}
	return declaration, deviceKey, nil
}

// Close ends pending waits and releases the Root reference.
func (w *workerDeclarationWatch) Close() { w.release() }
