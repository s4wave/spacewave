package provider_local

import (
	"net/url"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if _, err := c.ParsePeerID(); err != nil {
		return errors.Wrap(err, "peer_id")
	}
	if _, err := c.ParseSignalingURL(); err != nil {
		return errors.Wrap(err, "signaling_url")
	}
	return nil
}

// ParseSignalingURL accepts an absolute HTTP(S) URL or an origin-relative path
// for browser signaling. Empty disables signaling for standalone local sessions.
func (c *Config) ParseSignalingURL() (*url.URL, error) {
	raw := strings.TrimSpace(c.GetSignalingUrl())
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" && u.Host == "" && strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return u, nil
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return nil, errors.Errorf("must be an absolute http or https URL or an origin-relative path, got %q", raw)
	}
	if u.Host == "" {
		return nil, errors.Errorf("must be an absolute http or https URL or an origin-relative path, got %q", raw)
	}
	return u, nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// ParsePeerID parses the peer id field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
