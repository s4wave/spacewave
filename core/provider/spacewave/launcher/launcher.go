package spacewave_launcher

import (
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

// ChannelStable is the default release channel. A DistConfig with an empty
// Channel field is treated as stable, so legacy configs and older signers
// keep working without explicit upgrades.
const ChannelStable = "stable"

// ResolvedChannel returns the DistConfig channel with the empty-string
// fallback already applied. Use this instead of reading GetChannel() directly
// so the "empty == stable" rule lives in one place.
func (c *DistConfig) ResolvedChannel() string {
	if ch := c.GetChannel(); ch != "" {
		return ch
	}
	return ChannelStable
}

// Validate performs basic validation of the config.
func (c *DistConfig) Validate() error {
	if len(c.GetProjectId()) == 0 {
		return errors.New("project id cannot be empty")
	}
	if c.GetRev() == 0 {
		return errors.New("rev cannot be empty")
	}
	return nil
}

// UnmarshalFromYAML unmarshals the configuration from yaml.
func (c *DistConfig) UnmarshalFromYAML(dat []byte) error {
	jdat, err := yaml.YAMLToJSON(dat)
	if err != nil {
		return err
	}
	return c.UnmarshalJSON(jdat)
}

// MarshalToJSON marshals the configuration to json.
func (c *DistConfig) MarshalToJSON() ([]byte, error) {
	return c.MarshalJSON()
}

// MarshalToYAML marshals the configuration to yaml.
func (c *DistConfig) MarshalToYAML() ([]byte, error) {
	jdat, err := c.MarshalToJSON()
	if err != nil {
		return nil, err
	}

	return yaml.JSONToYAML(jdat)
}
