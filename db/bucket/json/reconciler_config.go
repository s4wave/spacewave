package bucket_json

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
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

func parseReconcilerConfigValue(v *fastjson.Value) (ReconcilerConfig, error) {
	if v == nil || v.Type() != fastjson.TypeObject {
		return ReconcilerConfig{}, errors.New("reconciler config must be object")
	}

	controller, err := parseControllerConfigValue(v.Get("controller"))
	if err != nil {
		return ReconcilerConfig{}, err
	}

	return ReconcilerConfig{
		Id:         string(v.GetStringBytes("id")),
		Controller: controller,
	}, nil
}

func (c *ReconcilerConfig) marshalJSONValue(a *fastjson.Arena) (*fastjson.Value, error) {
	obj := a.NewObject()
	obj.Set("id", a.NewString(c.Id))

	controller, err := marshalControllerConfigValue(a, c.Controller)
	if err != nil {
		return nil, err
	}
	obj.Set("controller", controller)
	return obj, nil
}
