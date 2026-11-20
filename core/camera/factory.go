//go:build !js

package camera

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/s4wave/spacewave/core/camera/v4l2"
	frametrack_server "github.com/s4wave/spacewave/core/frametrack/server"
	"github.com/s4wave/spacewave/core/testpattern"
)

// version is the camera controller version.
var version = controller.MustParseVersion("0.0.1")

// Factory builds the camera controller on a Session bus.
type Factory struct {
	// b is the Session bus serving camera streams.
	b bus.Bus
}

// NewFactory builds a camera factory for the Session bus.
func NewFactory(b bus.Bus) *Factory {
	return &Factory{b: b}
}

// GetConfigID returns the configuration ID the factory accepts.
func (f *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the camera controller ID.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs the camera controller configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct creates the controller that serves one track while it executes.
func (f *Factory) Construct(_ context.Context, conf config.Config, opts controller.ConstructOpts) (controller.Controller, error) {
	// Bind the configured camera source to this Session bus.
	device := conf.(*Config).GetDevice()
	info := controller.NewInfo(ControllerID, version, "device camera controller")
	le := opts.GetLogger().WithField("controller", ControllerID)

	// Open one view-only source for this controller execution.
	return callback.NewCallbackController(info, func(ctx context.Context) error {
		// With no device configured, serve the built-in JPEG pattern.
		if device == "" {
			return frametrack_server.Serve(ctx, le, f.b, ControlProtocolID, testpattern.NewJPEGSource(), 1, nil)
		}

		// Serve the device's negotiated compressed frames.
		source, err := v4l2.NewSource(device)
		if err != nil {
			return err
		}
		// Leave room for H.264 deltas behind a keyframe in flight.
		return frametrack_server.Serve(ctx, le, f.b, ControlProtocolID, source, 4, nil)
	}, nil, nil), nil
}

// GetVersion returns the camera controller version.
func (f *Factory) GetVersion() controller.Version {
	return version
}

// _ is a type assertion.
var _ controller.Factory = (*Factory)(nil)
