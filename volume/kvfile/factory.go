package volume_kvfile

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/hydra/volume"
	vc "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Factory constructs a in-memory volume.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a in-memory volume factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// NewVolumeController builds a new kvfile volume and volume controller.
//
// filePath is ignored and the given reader is used instead.
func NewVolumeController(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	cc *Config,
	rdr *kvfile.Reader,
) (*vc.Controller, error) {
	// Construct the volume controller.
	return vc.NewController(
		le,
		cc.GetVolumeConfig(),
		b,
		controller.NewInfo(
			ControllerID,
			Version,
			ctrlDescrip,
		),
		func(
			ctx context.Context,
			le *logrus.Entry,
		) (volume.Volume, error) {
			return NewKVFile(
				ctx,
				le,
				cc,
				rdr,
			)
		},
	), nil
}

// GetConfigID returns the unique ID for the config.
func (t *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (t *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated controller given configuration.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	// Construct the volume controller.
	return vc.NewController(
		le,
		cc.GetVolumeConfig(),
		t.bus,
		controller.NewInfo(
			ControllerID,
			Version,
			ctrlDescrip,
		),
		func(
			ctx context.Context,
			le *logrus.Entry,
		) (volume.Volume, error) {
			// open the configured file
			f, err := os.OpenFile(cc.GetFilePath(), os.O_RDONLY, 0o644)
			if err != nil {
				return nil, err
			}

			rdr, err := kvfile.BuildReaderWithFile(f)
			if err != nil {
				_ = f.Close()
				return nil, err
			}

			return NewKVFile(
				ctx,
				le,
				cc,
				rdr,
			)
		},
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
