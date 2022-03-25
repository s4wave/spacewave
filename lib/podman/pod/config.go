package forge_lib_podman_pod

import (
	"strings"

	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	k8s_v1 "k8s.io/api/core/v1"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	spec := &k8s_v1.PodSpec{}
	err := c.ParseSpec(spec)
	if err != nil {
		return errors.Wrap(err, "spec")
	}
	_, err = c.BuildVolumeMap(spec)
	if err != nil {
		return err
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return proto.Equal(c, other)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *Config) MarshalBlock() ([]byte, error) {
	return proto.Marshal(c)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *Config) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, c)
}

// ParseSpec parses the pod spec field.
func (c *Config) ParseSpec(spec *k8s_v1.PodSpec) error {
	return ParsePodSpec([]byte(c.GetSpec()), spec)
}

// BuildVolumeMap maps the k8s pod volumes to world volumes.
//
// HostPath: the host path must match match a world volume id.
// PersistentVolumeClaim: claimName must match the world volume id.
//
// Any other volume type is disallowed.
func (c *Config) BuildVolumeMap(spec *k8s_v1.PodSpec) (map[string][]*k8s_v1.Volume, error) {
	out := make(map[string][]*k8s_v1.Volume)
	worldVolumes := c.GetWorldVolumes()
	if worldVolumes == nil {
		worldVolumes = make(map[string]*WorldVolume)
	}

	for k, v := range worldVolumes {
		if err := labels.ValidateDNSLabel(k); err != nil {
			return nil, errors.Errorf("world_volumes: invalid label: %s", k)
		}
		if err := v.Validate(); err != nil {
			return nil, errors.Wrapf(err, "world_volumes[%s]", k)
		}
	}

	for i := range spec.Volumes {
		vol := &spec.Volumes[i]
		volName := vol.Name
		if volName == "" {
			return nil, errors.Errorf("volumes[%d]: name must be set", i)
		}

		var worldVolName string
		if hp := vol.HostPath; hp != nil && hp.Path != "" {
			hpPath := hp.Path

			// Podman checks if the field contains a / to see if it's a path.
			if strings.Contains(hpPath, "/") {
				return nil, errors.Errorf("volumes[%d]: host path not allowed: %s", i, hpPath)
			}

			// Otherwise the path must match one of the world volumes.
			worldVolName = hpPath
		}

		if pvc := vol.PersistentVolumeClaim; pvc != nil && worldVolName == "" {
			worldVolName = pvc.ClaimName
		}

		if worldVolName == "" {
			return nil, errors.Errorf("volumes[%d]: %s: unsupported volume type", i, volName)
		}

		if _, ok := worldVolumes[worldVolName]; !ok {
			return nil, errors.Errorf("volumes[%d]: host path must match a world volume name: %s", i, worldVolName)
		}

		out[worldVolName] = append(out[worldVolName], vol)
	}

	return out, nil
}

// Validate checks the WorldVolume.
func (v *WorldVolume) Validate() error {
	return v.ToUnixfsRef().Validate()
}

// ToUnixfsRef converts the WorldVolume to a UnixfsRef.
func (v *WorldVolume) ToUnixfsRef() *unixfs_world.UnixfsRef {
	return &unixfs_world.UnixfsRef{
		ObjectKey: v.GetObjectKey(),
		FsType:    v.GetFsType(),
		Path:      unixfs_block.SplitFSPath(v.GetPath()),
	}
}

// _ is a type assertion
var (
	_ config.Config = ((*Config)(nil))
	_ block.Block   = ((*Config)(nil))
)
