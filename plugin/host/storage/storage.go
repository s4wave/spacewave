package plugin_host_storage

import (
	plugin_host_storage_volume "github.com/aperturerobotics/bldr/plugin/host/storage/volume"
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
)

// PluginHostStorage provides storage via the plugin host.
type PluginHostStorage struct{}

// NewPluginHostStorage constructs the storage.
func NewPluginHostStorage() *PluginHostStorage {
	return &PluginHostStorage{}
}

// GetStorageInfo returns StorageInfo.
func (s *PluginHostStorage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *PluginHostStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_rpc_client.NewFactory(b))
	sr.AddFactory(plugin_host_storage_volume.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
// baseVolCtrlConf can be nil
func (s *PluginHostStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return &plugin_host_storage_volume.Config{StorageVolumeId: id, VolumeConfig: baseVolCtrlConf}, nil
}

// _ is a type assertion
var _ storage.Storage = ((*PluginHostStorage)(nil))
