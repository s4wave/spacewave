package devtool_web_entrypoint_plugin_host

import (
	"github.com/pkg/errors"
)

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig() *Config {
	return &Config{}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.GetFetchBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "fetch_backoff")
	}
	if err := c.GetExecBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "exec_backoff")
	}
	return nil
}
