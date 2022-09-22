package plugin_host_process

import (
	"errors"
	"path"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(engineID, objectKey string, peerID peer.ID, stateDir, distDir string) *Config {
	return &Config{
		EngineId:  engineID,
		ObjectKey: objectKey,
		PeerId:    peerID.Pretty(),

		StateDir: stateDir,
		DistDir:  distDir,
	}
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
// Example: controllerbus/example/boilerplate/1
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.ToControllerConfig().Validate(); err != nil {
		return err
	}
	if !path.IsAbs(c.GetStateDir()) {
		return errors.New("state dir: must be absolute path")
	}
	if !path.IsAbs(c.GetDistDir()) {
		return errors.New("dist dir: must be absolute path")
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// ToControllerConfig builds the controller config.
func (c *Config) ToControllerConfig() *plugin_host_controller.Config {
	return plugin_host_controller.NewConfig(
		c.GetEngineId(),
		c.GetObjectKey(),
		c.GetPeerId(),
	)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
