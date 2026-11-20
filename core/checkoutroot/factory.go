//go:build !js

package checkoutroot

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/go-git/go-billy/v6/osfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// version is the checkout-root controller version.
var version = controller.MustParseVersion("0.0.1")

// Factory builds checkout-root controllers on the Device's Session bus.
type Factory struct {
	// b is the Session bus that owns incoming links.
	b bus.Bus
	// stateVolumeID identifies the daemon's local state volume on the parent bus.
	stateVolumeID string
	// publications serializes replacing uploads across the Device's Session buses.
	publications *sync.Mutex
}

// NewFactory builds checkout roots on the Session bus with the daemon's local state volume.
func NewFactory(b bus.Bus, stateVolumeID string, publications *sync.Mutex) *Factory {
	return &Factory{b: b, stateVolumeID: stateVolumeID, publications: publications}
}

// GetConfigID returns the configuration ID the factory accepts.
func (f *Factory) GetConfigID() string { return ConfigID }

// GetControllerID returns the controller ID the factory builds.
func (f *Factory) GetControllerID() string { return ControllerID }

// GetVersion returns the controller version.
func (f *Factory) GetVersion() controller.Version { return version }

// ConstructConfig creates an empty checkout-root configuration for decoding.
func (f *Factory) ConstructConfig() config.Config { return &Config{} }

// Construct opens the configured folder and serves it for the controller's lifetime.
func (f *Factory) Construct(ctx context.Context, config config.Config, opts controller.ConstructOpts) (controller.Controller, error) {
	// Validate the configuration before opening anything on the Device.
	conf := config.(*Config)
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	// Pin the directory so every operation stays within the same host root.
	root, err := os.OpenRoot(conf.GetPath())
	if err != nil {
		return nil, err
	}
	filesystem, err := osfs.FromRoot(root)
	if err != nil {
		_ = root.Close()
		return nil, err
	}

	// Resolve the daemon state volume before advertising a writable root.
	info := controller.NewInfo(ControllerID, version, "device checkout-root controller")
	le := opts.GetLogger().WithField("controller", ControllerID)
	id := conf.ProtocolID()
	closeRoot := root.Close
	var journal *uploadJournal
	var upload unixfs_rpc_server.FileUpload
	if conf.GetWritable() {
		vol, _, ref, err := volume.ExLookupVolume(ctx, f.b, f.stateVolumeID, "", false)
		if err != nil {
			_ = root.Close()
			err = fmt.Errorf("resolve checkout-root state volume %q: %w", f.stateVolumeID, err)
			if ctx.Err() == nil {
				le.WithError(err).Error("open checkout-root upload journal")
			}
			return nil, err
		}
		kv, ok := vol.(volume_kvtx.KvtxVolume)
		if !ok {
			ref.Release()
			_ = root.Close()
			err := fmt.Errorf("checkout-root state volume %q has no key/value store", f.stateVolumeID)
			le.WithError(err).Error("open checkout-root upload journal")
			return nil, err
		}

		// Hold the state volume as long as the root can publish or recover uploads.
		journal = newUploadJournal(kv.GetKvtxStore(), id)
		closeRoot = func() error {
			defer ref.Release()
			return root.Close()
		}
		upload = func(ctx context.Context, dir string, start *unixfs_rpc.OpsMknodWithContentStart, src io.Reader) error {
			return publishFile(ctx, root, journal, f.publications, dir, start, src)
		}
	}

	// Publish the UnixFS handler only for this controller's execution.
	cursor := unixfs_billy.NewVersionedBillyFSCursor(filesystem, "", readHostFileVersion)
	return callback.NewCallbackController(info, func(ctx context.Context) error {
		return serve(ctx, le, f.b, id, root, cursor, upload, journal)
	}, nil, closeRoot), nil
}

// _ is a type assertion.
var _ controller.Factory = (*Factory)(nil)
