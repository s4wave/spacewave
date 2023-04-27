package storage_cli

import (
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	hcli "github.com/aperturerobotics/hydra/cli"
	hydra_all "github.com/aperturerobotics/hydra/core/all"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
)

// CliStorage wraps cli args to provide storage.
type CliStorage struct {
	args *hcli.DaemonArgs
}

// NewCliStorage constructs storage from CLI args.
func NewCliStorage(args *hcli.DaemonArgs) *CliStorage {
	return &CliStorage{args: args}
}

// GetStorageInfo returns StorageInfo.
func (s *CliStorage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{
		Isolated: false,
		Cache:    false,
	}
}

// AddFactories adds the factories to the resolver.
func (s *CliStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	hydra_all.AddFactories(b, sr)
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
// baseVolCtrlConf can be nil
func (s *CliStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) config.Config {
	return s.args.BuildSingleVolume(baseVolCtrlConf)
}

// _ is a type assertion
var _ storage.Storage = ((*CliStorage)(nil))
