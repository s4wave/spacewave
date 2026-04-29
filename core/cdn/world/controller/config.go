package cdn_world_controller

import (
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(engineID, spaceID, cdnBaseURL string) *Config {
	return &Config{
		EngineId:   engineID,
		SpaceId:    spaceID,
		CdnBaseUrl: cdnBaseURL,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetEngineId() == "" {
		return errors.New("engine_id cannot be empty")
	}
	if c.GetSpaceId() == "" {
		return errors.New("space_id cannot be empty")
	}
	if c.GetCdnBaseUrl() == "" {
		return errors.New("cdn_base_url cannot be empty")
	}
	if _, err := c.ParsePointerTTLDur(); err != nil {
		return errors.Wrap(err, "pointer_ttl_dur")
	}
	return nil
}

// ParsePointerTTLDur parses the root pointer TTL field.
func (c *Config) ParsePointerTTLDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetPointerTtlDur())
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
