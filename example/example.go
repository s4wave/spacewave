package bldr_example

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "bldr/example/demo"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "demo controller"

// Demo is a demo controller.
type Demo struct {
	*bus.BusController[*Config]
}

// NewDemo constructs a new demo controller.
func NewDemo(le *logrus.Entry, b bus.Bus, conf *Config) (*Demo, error) {
	return &Demo{
		BusController: bus.NewBusController(le, b, conf, ControllerID, Version, controllerDescrip),
	}, nil
}

// NewFactory constructs the demo controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Demo, error) {
			return &Demo{BusController: base}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (d *Demo) Execute(ctx context.Context) error {
	le := d.GetLogger()
	le.Info("hello from the bldr example demo controller")

	vol, volRef, err := volume.ExLookupVolume(ctx, d.GetBus(), plugin.PluginVolumeID, "")
	if err != nil {
		return err
	}
	if volRef == nil {
		return errors.New("look up host volume returned not found")
	}
	defer volRef.Release()

	le.Info("successfully looked up volume")

	le.Info("testing object store api")
	if err := store_test.TestObjectStore(ctx, vol); err != nil {
		return err
	}

	le.Info("testing message queue api")
	if err := store_test.TestMqueueAPI(ctx, vol); err != nil {
		return err
	}

	le.Info("volume tests passed")
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Demo)(nil)
