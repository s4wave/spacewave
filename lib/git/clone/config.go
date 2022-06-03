package forge_lib_git_clone

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := c.GetCloneOpts().Validate(); err != nil {
		return errors.Wrap(err, "clone_opts")
	}
	if err := c.GetAuthOpts().Validate(); err != nil {
		return errors.Wrap(err, "auth_opts")
	}
	if err := c.GetWorktreeOpts().Validate(); err != nil {
		return errors.Wrap(err, "worktree_opts")
	}
	return nil
}

// IsEmpty checks if there are no operations in the config.
func (c *Config) IsEmpty() bool {
	return len(c.GetObjectKey()) == 0 || c.GetCloneOpts().IsEmpty()
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
