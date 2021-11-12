package bucket_json

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
)

// LookupConfig implements the lookup configuration JSON marshalling logic.
type LookupConfig struct {
	// Disble indicates we should not service cross-volume calls against this
	// bucket.
	Disable bool `json:"disable"`
	// Controller is the controller configuration.
	Controller *configset_json.ControllerConfig `json:"controller"`
}

// NewLookupConfig builds a new lookup config.
func NewLookupConfig(disable bool, config configset.ControllerConfig) *LookupConfig {
	return &LookupConfig{
		Disable:    disable,
		Controller: configset_json.NewControllerConfig(config),
	}
}

// GetDisable returns the disable field.
func (c *LookupConfig) GetDisable() bool {
	return c.Disable
}
