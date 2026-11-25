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

// Validate requires the placed capability and a codec the source can serve.
func (c *Config) Validate() error {
	// The protocol must belong to one placed node through its lifetime.
	if c.GetCapabilityId() == "" {
		return errors.New("display capability ID is required")
	}

	// Match the rendition to the selected source.
	codec := c.GetCodec()
	switch c.GetSource() {
	case Source_SOURCE_UNKNOWN:
		if codec == frametrack.Codec_CODEC_H264_AVC || codec == frametrack.Codec_CODEC_JPEG {
			return nil
		}
		return errors.New("display codec must be h264 or jpeg")
	case Source_SOURCE_GSTREAMER_TEST, Source_SOURCE_PLASMA:
		if codec == frametrack.Codec_CODEC_H264_AVC {
			return nil
		}
		return errors.New("GStreamer display capture requires h264")
	default:
		return errors.New("unknown display source")
	}
}

// GetConfigID returns the display controller's configuration ID.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig reports whether other selects the same display configuration.
func (c *Config) EqualsConfig(other config.Config) bool {
	o, ok := other.(*Config)
	return ok && c.EqualVT(o)
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
