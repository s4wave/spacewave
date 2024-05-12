package storage

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
)

// Storage is an available storage mechanism in the environment.
type Storage interface {
	// GetStorageInfo returns StorageInfo.
	GetStorageInfo() *StorageInfo
	// AddFactories adds the factories to the resolver.
	AddFactories(b bus.Bus, sr *static.Resolver)
	// BuildVolumeConfig creates the volume config for the store ID.
	// Returns nil if the storage cannot produce Volume.
	// baseVolCtrlConf can be nil
	// NOTE: id should be checked / sanitized before calling this.
	BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error)
}
