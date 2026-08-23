package volume_filesnap

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	vc "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the snapshot-file volume controller.
const ControllerID = "hydra/volume/filesnap"

// Version is the version of the implementation.
var Version = controller.MustParseVersion("0.0.1")

// Volume is a snapshot-file backed volume.
type Volume = common_kvtx.Volume

// NewFileSnap builds a new snapshot-file volume, loading the file.
func NewFileSnap(ctx context.Context, le *logrus.Entry, path string) (*Volume, error) {
	kvkey, err := kvkey.NewKVKey(nil)
	if err != nil {
		return nil, err
	}
	store, err := NewStore(path)
	if err != nil {
		return nil, err
	}
	return common_kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		store,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
}

// store_kvtx_config is a zero store config.
type store_kvtx_config struct{}

func (c *store_kvtx_config) GetVerbose() bool { return false }

// Factory constructs a snapshot-file volume.
type Factory struct {
	bus bus.Bus
}

// NewFactory builds a snapshot-file volume factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the unique ID for the config.
func (t *Factory) GetConfigID() string { return ConfigID }

// GetControllerID returns the unique ID for the controller.
func (t *Factory) GetControllerID() string { return ControllerID }

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config { return &Config{} }

// Construct constructs the associated controller given configuration.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	return vc.NewController(
		le,
		cc.GetVolumeConfig(),
		t.bus,
		controller.NewInfo(
			ControllerID,
			Version,
			"snapshot-file kvtx",
		),
		func(
			ctx context.Context,
			le *logrus.Entry,
		) (volume.Volume, error) {
			return NewFileSnap(ctx, le, cc.Path)
		},
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version { return Version }

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
