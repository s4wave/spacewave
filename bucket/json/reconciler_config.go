package bucket_json

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
)

// ReconcilerConfig implements the reconciler configuration JSON marshalling logic.
type ReconcilerConfig struct {
	// Id is the reconciler ID.
	Id string `json:"id"`
	// Controller is the controller configuration.
	Controller *configset_json.ControllerConfig `json:"controller"`
}

// NewReconcilerConfig builds a new controller config.
func NewReconcilerConfig(id string, config configset.ControllerConfig) *ReconcilerConfig {
	return &ReconcilerConfig{
		Id:         id,
		Controller: configset_json.NewControllerConfig(config),
	}
}

// GetId returns the id.
func (c *ReconcilerConfig) GetId() string {
	return c.Id
}
