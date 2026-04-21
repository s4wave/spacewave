//go:build !js

package storage_cli

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/bldr/storage"
	hcli "github.com/s4wave/spacewave/db/cli"
	hydra_cli_core "github.com/s4wave/spacewave/db/cli/core"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
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
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *CliStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	hydra_cli_core.AddFactories(b, sr)
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
// baseVolCtrlConf can be nil
func (s *CliStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return s.args.BuildSingleVolume(id, baseVolCtrlConf), nil
}

// DeleteVolume is not supported for CLI storage.
func (s *CliStorage) DeleteVolume(id string) error {
	return nil
}

// _ is a type assertion
var _ storage.Storage = ((*CliStorage)(nil))
