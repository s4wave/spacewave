package bucket

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

// NewReconcilerConfig builds a new controller config.
func NewReconcilerConfig(id string, config configset.ControllerConfig) (*ReconcilerConfig, error) {
	c, err := configset_proto.NewControllerConfig(config)
	if err != nil {
		return nil, err
	}

	return &ReconcilerConfig{
		Id:         id,
		Controller: c,
	}, nil
}
