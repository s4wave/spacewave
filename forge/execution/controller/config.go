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
	// Hash the World, peer, Execution, and Worker as the controller identity.
	h := blake3.NewDeriveKey("forge/execution/controller: config: unique id")
	for _, part := range []string{c.GetEngineId(), c.GetPeerId(), c.GetObjectKey(), c.GetWorkerObjectKey()} {
		_, _ = h.WriteString(part)
		_, _ = h.WriteString("\x00")
	}

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

// ParseClaimLease parses a lease longer than the clock-skew allowance.
func (c *Config) ParseClaimLease() (time.Duration, error) {
	// Use the default lease when none is configured.
	leaseStr := c.GetClaimLease()
	if leaseStr == "" {
		return forge_execution.DefaultClaimLease, nil
	}

	// Leave time for execution before the claimant's early fencing deadline.
	lease, err := time.ParseDuration(leaseStr)
	if err != nil {
		return 0, err
	}
	if lease <= forge_execution.ClaimClockSkew {
		return 0, errors.New("claim lease must exceed the clock-skew allowance")
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
