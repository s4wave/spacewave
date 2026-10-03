package execution_controller

import (
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	uuid "github.com/satori/go.uuid"
	"github.com/zeebo/blake3"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new execution controller config.
// Sets the most important fields only.
func NewConfig(engineID, objectKey string, peerID peer.ID, inpWorld *forge_target.InputWorld) *Config {
	// Encode the optional peer identity for the Execution configuration.
	var peerIDStr string
	if peerID != "" {
		peerIDStr = peerID.String()
	}

	// Bind the World object and input World to a durable execution claim.
	conf := &Config{
		EngineId:  engineID,
		ObjectKey: objectKey,
		PeerId:    peerIDStr,

		InputWorld: inpWorld,
	}
	conf.ClaimId = conf.BuildUniqueID()
	return conf
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	// Require a World engine before resolving the Execution object.
	if len(c.GetEngineId()) == 0 {
		return errors.New("world engine id must be specified")
	}

	// Require the Execution object key within the configured World.
	if len(c.GetObjectKey()) == 0 {
		return errors.New("world object key must be specified")
	}

	// Validate the optional peer identity used to execute the target.
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}

	// Validate the optional deadline for controller configuration resolution.
	if _, err := c.ParseResolveControllerConfigTimeout(); err != nil {
		return err
	}

	// Validate the optional claim lease duration.
	if _, err := c.ParseClaimLease(); err != nil {
		return err
	}
	return nil
}

// BuildUniqueID builds the durable execution controller ID.
func (c *Config) BuildUniqueID() string {
	// Hash the World engine, peer and object key as the Execution identity.
	h := blake3.NewDeriveKey("forge/execution/controller: config: unique id")
	_, _ = h.WriteString(c.GetEngineId())
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(c.GetPeerId())
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(c.GetObjectKey())

	// Encode the Execution identity hash as a UUID.
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

// ParseClaimLease parses the claim lease duration, defaulting when unset.
func (c *Config) ParseClaimLease() (time.Duration, error) {
	// Use the default lease when none is configured.
	leaseStr := c.GetClaimLease()
	if leaseStr == "" {
		return forge_execution.DefaultClaimLease, nil
	}

	// Require a positive duration.
	lease, err := time.ParseDuration(leaseStr)
	if err != nil {
		return 0, err
	}
	if lease <= 0 {
		return 0, errors.New("claim lease must be positive")
	}
	return lease, nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
