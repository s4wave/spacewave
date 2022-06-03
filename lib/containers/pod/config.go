package forge_lib_containers_pod

import (
	"github.com/aperturerobotics/containers/pod"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	yaml "sigs.k8s.io/yaml"

	k8s_v1 "k8s.io/api/core/v1"
	k8s_metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetEngineId() == "" {
		return pod.ErrEmptyEngineID
	}
	if _, err := c.ParsePod(); err != nil {
		return err
	}

	return nil
}

// ParsePod parses the pod spec and metadata.
func (c *Config) ParsePod() (*k8s_v1.Pod, error) {
	out := &k8s_v1.Pod{}

	if err := c.ParseMeta(&out.ObjectMeta); err != nil {
		return nil, errors.Wrap(err, "meta")
	}

	err := c.GetPod().ParseSpec(&out.Spec)
	if err != nil {
		return nil, errors.Wrap(err, "spec")
	}

	return out, nil
}

// ParseMeta parses the object metadata.
func (c *Config) ParseMeta(out *k8s_metav1.ObjectMeta) error {
	if out == nil {
		return nil
	}
	if metaYAML := c.GetMeta(); metaYAML != "" {
		if err := yaml.Unmarshal([]byte(metaYAML), out); err != nil {
			return err
		}
	}
	if name := c.GetName(); name != "" {
		out.Name = name
	}
	if genName := c.GetGenerateName(); genName != "" {
		out.GenerateName = genName
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

// _ is a type assertion
var (
	_ config.Config = ((*Config)(nil))
	_ block.Block   = ((*Config)(nil))
)
