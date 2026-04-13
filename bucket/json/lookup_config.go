package bucket_json

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
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

func parseLookupConfigValue(v *fastjson.Value) (*LookupConfig, error) {
	if v == nil || v.Type() == fastjson.TypeNull {
		return nil, nil
	}
	if v.Type() != fastjson.TypeObject {
		return nil, errors.New("lookup config must be object")
	}

	controller, err := parseControllerConfigValue(v.Get("controller"))
	if err != nil {
		return nil, err
	}

	return &LookupConfig{
		Disable:    v.GetBool("disable"),
		Controller: controller,
	}, nil
}

func (c *LookupConfig) marshalJSONValue(a *fastjson.Arena) (*fastjson.Value, error) {
	if c == nil {
		return a.NewNull(), nil
	}

	obj := a.NewObject()
	if c.Disable {
		obj.Set("disable", a.NewTrue())
	} else {
		obj.Set("disable", a.NewFalse())
	}

	controller, err := marshalControllerConfigValue(a, c.Controller)
	if err != nil {
		return nil, err
	}
	obj.Set("controller", controller)
	return obj, nil
}
