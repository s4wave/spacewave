package forge_lib_git_commit

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetWorktreeObjectKey() == "" {
		return errors.New("worktree_object_key cannot be empty")
	}
	if c.GetRepoObjectKey() == "" {
		return errors.New("repo_object_key cannot be empty")
	}
	req := c.GetCommitRequest()
	if req == nil {
		return errors.New("commit_request cannot be empty")
	}
	if req.GetMessage() == "" {
		return errors.New("commit_request.message cannot be empty")
	}
	if req.GetAuthorName() == "" {
		return errors.New("commit_request.author_name cannot be empty")
	}
	if req.GetAuthorEmail() == "" {
		return errors.New("commit_request.author_email cannot be empty")
	}
	return nil
}

// IsEmpty checks if there are no operations in the config.
func (c *Config) IsEmpty() bool {
	return c.GetWorktreeObjectKey() == "" || c.GetCommitRequest() == nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *Config) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *Config) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// _ is a type assertion
var (
	_ config.Config = ((*Config)(nil))
	_ block.Block   = ((*Config)(nil))
)
