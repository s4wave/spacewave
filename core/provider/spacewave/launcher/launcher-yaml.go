//go:build !tinygo && !goscript

package spacewave_launcher

import "github.com/ghodss/yaml"

// UnmarshalFromYAML unmarshals the configuration from yaml.
func (c *DistConfig) UnmarshalFromYAML(dat []byte) error {
	jdat, err := yaml.YAMLToJSON(dat)
	if err != nil {
		return err
	}
	return c.UnmarshalJSON(jdat)
}

// MarshalToYAML marshals the configuration to yaml.
func (c *DistConfig) MarshalToYAML() ([]byte, error) {
	jdat, err := c.MarshalToJSON()
	if err != nil {
		return nil, err
	}

	return yaml.JSONToYAML(jdat)
}
