// Package state defines the durable state shared by native entrypoints.
package state

import (
	"github.com/aperturerobotics/controllerbus/config"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
)

// VolumeID names the single entrypoint state volume beneath a state root.
const VolumeID = "state"

// Filename is the native state volume filename used by Session file selectors.
const Filename = VolumeID + ".s4wave"

// NewVolumeConfig mounts the daemon's store under the host and distribution aliases.
func NewVolumeConfig(storageID string) *storage_volume.Config {
	return &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: VolumeID,
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias:       []string{bldr_plugin.PluginVolumeID, "dist"},
			DisableEventBlockRm: true,
			GcIntervalDur:       "0",
		},
	}
}

// NewWorldConfig opens the same durable application World for every renderer.
func NewWorldConfig(projectID, volumeID string) (*world_block_engine.Config, error) {
	// Use one block encoding for native CLI and distribution entrypoints.
	transformConf, err := block_transform.NewConfig([]config.Config{&transform_s2.Config{}})
	if err != nil {
		return nil, err
	}

	// Store the project World head beside its blocks in the shared volume.
	engineID := "entrypoint/" + projectID
	return world_block_engine.NewConfig(
		engineID, volumeID, engineID, engineID,
		&bucket.ObjectRef{BucketId: engineID, TransformConf: transformConf},
		nil, false,
	), nil
}
