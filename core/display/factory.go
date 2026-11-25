//go:build !js

package display

import (
	"context"
	"image"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/s4wave/spacewave/core/display/gstencoder"
	"github.com/s4wave/spacewave/core/frametrack"
	frametrack_server "github.com/s4wave/spacewave/core/frametrack/server"
	"github.com/s4wave/spacewave/core/testpattern"
)

// version is the display controller version.
var version = controller.MustParseVersion("0.0.1")

// Factory builds the display controller on the Device's Session bus.
type Factory struct {
	// b is the Session bus serving display streams.
	b bus.Bus
}

// NewFactory builds the display factory for the Session bus.
func NewFactory(b bus.Bus) *Factory {
	return &Factory{b: b}
}

// GetConfigID returns the configuration ID the factory accepts.
func (f *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the display controller ID.
func (f *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs a display controller configuration for decoding.
func (f *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct creates the controller that runs the selected display source.
func (f *Factory) Construct(_ context.Context, config config.Config, opts controller.ConstructOpts) (controller.Controller, error) {
	// Validate the rendition before publishing the controller.
	conf := config.(*Config)
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	codec := conf.GetCodec()
	id := ControlProtocolID(conf.GetCapabilityId())
	info := controller.NewInfo(ControllerID, version, "device display controller")
	le := opts.GetLogger().WithField("controller", ControllerID)

	// Serve one source per controller execution. Only JPEG can repaint bars.
	return callback.NewCallbackController(info, func(ctx context.Context) error {
		// Bind Plasma capture and its track to the same portal session.
		if conf.GetSource() == Source_SOURCE_PLASMA {
			return servePlasma(ctx, le, f.b, id)
		}

		// Pass the GPU encoder's pictures straight through to the frame track.
		if conf.GetSource() == Source_SOURCE_GSTREAMER_TEST {
			source, err := gstencoder.OpenSource(ctx, gstencoder.Executable)
			if err != nil {
				return err
			}
			return frametrack_server.Serve(ctx, le, f.b, id, source, 4, frametrack_server.ObserveInput(func(*frametrack.Input) {}))
		}

		// The pre-encoded clip takes input without changing its pictures.
		if codec == frametrack.Codec_CODEC_H264_AVC {
			return frametrack_server.Serve(ctx, le, f.b, id, testpattern.NewH264Source(), 4, frametrack_server.ObserveInput(func(*frametrack.Input) {}))
		}

		// Draw the most recent pointer on the next JPEG from this source.
		source := testpattern.NewJPEGSource()
		return frametrack_server.Serve(ctx, le, f.b, id, source, 1, frametrack_server.ObserveInput(func(input *frametrack.Input) {
			if pointer := input.GetPointer(); pointer != nil {
				source.SetPointer(image.Pt(
					min(testpattern.Width-1, int(pointer.X*testpattern.Width)),
					min(testpattern.Height-1, int(pointer.Y*testpattern.Height)),
				))
			}
		}))
	}, nil, nil), nil
}

// GetVersion returns the display controller version.
func (f *Factory) GetVersion() controller.Version {
	return version
}

// _ is a type assertion.
var _ controller.Factory = (*Factory)(nil)
