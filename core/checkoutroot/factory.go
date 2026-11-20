//go:build !js

package checkoutroot

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/go-git/go-billy/v6/osfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

// version is the checkout-root controller version.
var version = controller.MustParseVersion("0.0.1")

// Factory builds checkout-root controllers on the Device's Session bus.
type Factory struct {
	// b is the Session bus that owns incoming links.
	b bus.Bus
}

// NewFactory builds the checkout-root factory for the Session bus.
func NewFactory(b bus.Bus) *Factory {
	return &Factory{b: b}
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
func (f *Factory) Construct(_ context.Context, config config.Config, opts controller.ConstructOpts) (controller.Controller, error) {
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

	// Publish the UnixFS handler only for this controller's execution.
	info := controller.NewInfo(ControllerID, version, "device checkout-root controller")
	le := opts.GetLogger().WithField("controller", ControllerID)
	cursor := unixfs_billy.NewBillyFSCursor(filesystem, "")
	return callback.NewCallbackController(info, func(ctx context.Context) error {
		return serve(ctx, le, f.b, conf.ProtocolID(), cursor, conf.GetWritable())
	}, nil, root.Close), nil
}

// _ is a type assertion.
var _ controller.Factory = (*Factory)(nil)
