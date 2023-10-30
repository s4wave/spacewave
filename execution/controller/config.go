package execution_controller

import (
	"errors"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	forge_target "github.com/aperturerobotics/forge/target"
	uuid "github.com/satori/go.uuid"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new execution controller config.
// Sets the most important fields only.
func NewConfig(engineID, objectKey string, peerID peer.ID, inpWorld *forge_target.InputWorld) *Config {
	var peerIDStr string
	if peerID != "" {
		peerIDStr = peerID.String()
	}
	return &Config{
		EngineId:  engineID,
		ObjectKey: objectKey,
		PeerId:    peerIDStr,

		InputWorld: inpWorld,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if len(c.GetEngineId()) == 0 {
		return errors.New("world engine id must be specified")
	}
	if len(c.GetObjectKey()) == 0 {
		return errors.New("world object key must be specified")
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if _, err := c.ParseResolveControllerConfigTimeout(); err != nil {
		return err
	}
	return nil
}

// BuildUniqueID builds the unique id for the execution instance.
func (c *Config) BuildUniqueID() string {
	h := blake3.NewDeriveKey("forge/execution/controller: config: unique id")
	h.WriteString(c.GetPeerId())
	h.WriteString(c.GetObjectKey())
	hsum := h.Sum(nil)
	var id uuid.UUID
	copy(id[:], hsum)
	return id.String()
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
