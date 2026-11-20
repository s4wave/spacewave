// Package display serves the Device's display to authorized account peers.
package display

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/net/protocol"
)

const (
	// ControllerID identifies the display controller.
	ControllerID = "spacewave/device/display"
	// ConfigID identifies the display controller configuration.
	ConfigID = ControllerID
)

// ControlProtocolID identifies the subscription stream of a placed display.
// A replacement node can start before the old handler is fully retired.
func ControlProtocolID(capabilityID string) protocol.ID {
	return protocol.ID("alpha/display/control/1/" + capabilityID)
}

// Validate requires the placed capability and a codec the test pattern serves.
func (c *Config) Validate() error {
	// The protocol must belong to one placed node through its lifetime.
	if c.GetCapabilityId() == "" {
		return errors.New("display capability ID is required")
	}

	// Choose a supported rendition for the source.
	switch c.GetCodec() {
	case frametrack.Codec_CODEC_H264_AVC, frametrack.Codec_CODEC_JPEG:
		return nil
	default:
		return errors.New("display codec must be h264 or jpeg")
	}
}

// GetConfigID returns the display controller's configuration ID.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig reports whether other selects the same test pattern.
func (c *Config) EqualsConfig(other config.Config) bool {
	o, ok := other.(*Config)
	return ok && c.EqualVT(o)
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
