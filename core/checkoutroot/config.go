// Package checkoutroot serves a Device folder to its account's Session peers.
package checkoutroot

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/protocol"
)

const (
	// ControllerID identifies the checkout-root controller.
	ControllerID = "spacewave/device/checkout-root"
	// ConfigID identifies the checkout-root controller configuration.
	ConfigID = ControllerID
)

// ProtocolID identifies the folder and access mode of this placement.
// A stream opened for a replaced configuration cannot reach a new handler.
func (c *Config) ProtocolID() protocol.ID {
	// Use the path and write mode to distinguish revisions of a placement.
	sum := sha256.Sum256([]byte(strconv.FormatBool(c.GetWritable()) + ":" + c.GetPath()))
	return protocol.ID("alpha/checkout-root/1/" + c.GetCapabilityId() + "/" + hex.EncodeToString(sum[:]))
}

// Validate requires a placed capability and an absolute folder path.
func (c *Config) Validate() error {
	// Identify the root by its placement and an unambiguous host path.
	if c.GetCapabilityId() == "" {
		return errors.New("checkout-root capability ID is required")
	}
	if !filepath.IsAbs(c.GetPath()) {
		return errors.New("checkout-root path must be absolute")
	}
	return nil
}

// GetConfigID returns the checkout-root configuration ID.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig reports whether other serves the same checkout root.
func (c *Config) EqualsConfig(other config.Config) bool {
	o, ok := other.(*Config)
	return ok && c.EqualVT(o)
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
