package execution_controller

import (
	"errors"
	"time"

	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/peer"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new execution controller config.
// Sets the most important fields only.
func NewConfig(engineID, objectID string, peerID peer.ID) *Config {
	var peerIDStr string
	if peerID != "" {
		peerIDStr = peerID.Pretty()
	}
	return &Config{
		EngineId: engineID,
		ObjectId: objectID,
		PeerId:   peerIDStr,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if len(c.GetEngineId()) == 0 {
		return errors.New("world engine id must be specified")
	}
	if len(c.GetObjectId()) == 0 {
		return errors.New("world object id must be specified")
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if _, err := c.ParseResolveControllerConfigTimeout(); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// ParseResolveControllerConfigTimeout parses the timeout dur.
func (c *Config) ParseResolveControllerConfigTimeout() (time.Duration, error) {
	timeoutStr := c.GetResolveControllerConfigTimeout()
	if timeoutStr == "" {
		return 0, nil
	}

	return time.ParseDuration(timeoutStr)
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

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
