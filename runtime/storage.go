package runtime

import "github.com/aperturerobotics/controllerbus/config"

// Storage is an available storage mechanism in the environment.
type Storage interface {
	// GetStorageInfo returns StorageInfo.
	GetStorageInfo() *StorageInfo
	// BuildVolumeConfig creates the volume config for the store ID.
	// Returns nil if the storage cannot produce Volume.
	BuildVolumeConfig(id string) config.Config
}
