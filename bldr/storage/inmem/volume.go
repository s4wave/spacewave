package storage_inmem

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/storage"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
)

// VolumeControllerID identifies the in-memory storage volume controller.
const VolumeControllerID = "bldr/storage/inmem/volume"

// Validate requires the owning storage and the volume id.
func (c *VolumeConfig) Validate() error {
	if c.GetStorageId() == "" {
		return errors.New("storage_id cannot be empty")
	}
	if c.GetStorageVolumeId() == "" {
		return errors.New("storage_volume_id cannot be empty")
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *VolumeConfig) GetConfigID() string {
	return VolumeControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *VolumeConfig) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*VolumeConfig](c, other)
}

// VolumeFactory constructs volumes whose data an InmemStorage on the bus owns.
type VolumeFactory struct {
	// b is the controller bus
	b bus.Bus
}

// NewVolumeFactory builds an in-memory storage volume factory.
func NewVolumeFactory(b bus.Bus) *VolumeFactory {
	return &VolumeFactory{b: b}
}

// GetConfigID returns the unique ID for the config.
func (f *VolumeFactory) GetConfigID() string {
	return VolumeControllerID
}

// GetControllerID returns the unique ID for the controller.
func (f *VolumeFactory) GetControllerID() string {
	return VolumeControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (f *VolumeFactory) ConstructConfig() config.Config {
	return &VolumeConfig{}
}

// Construct constructs the associated controller given configuration.
func (f *VolumeFactory) Construct(
	_ context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	cc, ok := conf.(*VolumeConfig)
	if !ok {
		return nil, errors.Errorf("expected config type %T, got %T", &VolumeConfig{}, conf)
	}
	if err := cc.Validate(); err != nil {
		return nil, err
	}

	return volume_controller.NewController(
		opts.GetLogger(),
		cc.GetVolumeConfig(),
		f.b,
		controller.NewInfo(VolumeControllerID, Version, "in-memory storage volume"),
		func(ctx context.Context, _ *logrus.Entry) (volume.Volume, error) {
			return f.openVolume(ctx, cc)
		},
	), nil
}

// GetVersion returns the version of this controller.
func (f *VolumeFactory) GetVersion() controller.Version {
	return Version
}

// openVolume finds the InmemStorage named by conf and opens the volume over
// its leased store. Closing the volume ends the lease.
func (f *VolumeFactory) openVolume(ctx context.Context, conf *VolumeConfig) (volume.Volume, error) {
	storages, _, ref, err := storage.ExLookupStorage(ctx, f.b, conf.GetStorageId(), true)
	if err != nil {
		return nil, err
	}
	ref.Release()

	var st *InmemStorage
	for _, candidate := range storages {
		if inmem, ok := candidate.(*InmemStorage); ok {
			st = inmem
			break
		}
	}
	if st == nil {
		return nil, errors.Errorf("no in-memory storage with id %q", conf.GetStorageId())
	}

	store, release, err := st.openVolume(ctx, conf.GetStorageVolumeId())
	if err != nil {
		return nil, err
	}
	kvkey, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		release()
		return nil, err
	}
	vol, err := common_kvtx.NewVolume(
		ctx,
		VolumeControllerID,
		kvkey,
		store,
		nil,
		false,
		false,
		nil,
		func() error {
			release()
			return nil
		},
	)
	if err != nil {
		release()
		return nil, err
	}
	return vol, nil
}

// _ is a type assertion
var (
	_ config.Config      = (*VolumeConfig)(nil)
	_ controller.Factory = (*VolumeFactory)(nil)
)
