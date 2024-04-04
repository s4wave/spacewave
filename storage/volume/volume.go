package storage_volume

import (
	"context"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the storage volume controller.
const ControllerID = "bldr/storage/volume"

// Version is the version of the redis implementation.
var Version = semver.MustParse("0.0.1")

// BuildVolumeControllerConfig builds a new storage volume config by accessing the Storage on the bus.
func BuildVolumeControllerConfig(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
) (config.Config, storage.Storage, error) {
	volumeConf, av, _, ref, err := bus.ExecOneOffWithXfrmTyped[storage.LookupStorageValue](
		ctx,
		b,
		storage.NewLookupStorage(conf.GetStorageId()),
		nil,
		nil,
		func(val directive.TypedAttachedValue[storage.LookupStorageValue]) (config.Config, bool, error) {
			// true above sets waitOne (wait for at least one)
			volConf := val.GetValue().BuildVolumeConfig(conf.GetStorageVolumeId(), conf.GetVolumeConfig())
			if volConf == nil {
				return nil, false, nil
			}
			return volConf, true, nil
		},
	)
	// we can release the LookupStorage once we match it.
	if ref != nil {
		ref.Release()
	}
	if err != nil {
		return nil, nil, err
	}

	// st is the selected storage
	return volumeConf, av.GetValue(), nil
}

// BuildVolumeController executes BuildVolumeControllerConfig followed by ExLoadFactoryByConfig then constructs the controller.
func BuildVolumeController(ctx context.Context, le *logrus.Entry, b bus.Bus, conf *Config) (volume.Controller, storage.Storage, error) {
	volConf, st, err := BuildVolumeControllerConfig(ctx, le, b, conf)
	if err != nil {
		return nil, nil, err
	}

	factory, factoryRef, err := resolver.ExLoadFactoryByConfig(ctx, b, volConf)
	if err != nil {
		return nil, nil, err
	}
	defer factoryRef.Release()

	ctrl, err := factory.Construct(ctx, volConf, controller.ConstructOpts{
		Logger: le,
	})
	if err != nil {
		return nil, nil, err
	}

	volCtrl, volCtrlOk := ctrl.(volume.Controller)
	if !volCtrlOk {
		_ = ctrl.Close()
		return nil, nil, errors.Errorf("storage %q returned volume config that was not a volume controller: %s", conf.GetStorageId(), volConf.GetConfigID())
	}

	return volCtrl, st, err
}
