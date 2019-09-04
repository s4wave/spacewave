package bucket_json

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/configset/json"
)

// PutOpts implements the put options JSON marshalling logic.
type PutOpts struct {
	// Id is the reconciler ID.
	Id string `json:"id"`
	// Controller is the controller configuration.
	Controller *configset_json.ControllerConfig `json:"controller"`
}

// NewPutOpts builds a new controller config.
func NewPutOpts(id string, config configset.ControllerConfig) *PutOpts {
	return &PutOpts{
		Id:         id,
		Controller: configset_json.NewControllerConfig(config),
	}
}

// GetId returns the id.
func (c *PutOpts) GetId() string {
	return c.Id
}
