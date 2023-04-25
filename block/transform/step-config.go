package block_transform

import (
	"encoding/base64"
	"encoding/json"

	gabs "github.com/Jeffail/gabs/v2"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// NewStepConfig constructs the step config with a underlying config.
func NewStepConfig(conf config.Config) (*StepConfig, error) {
	dat, err := proto.Marshal(conf)
	if err != nil {
		return nil, err
	}
	return &StepConfig{
		Id:     conf.GetConfigID(),
		Config: dat,
	}, nil
}

// UnmarshalStepConfig unmarshals a step config using either json or protobuf.
func UnmarshalStepConfig(data []byte, conf config.Config) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '{' {
		return jsonpb.Unmarshal(data, conf)
	}
	return conf.UnmarshalVT(data)
}

// IsNil returns if the object is nil.
func (c *StepConfig) IsNil() bool {
	return c == nil
}

// Validate performs cursory validation of the config.
func (c *StepConfig) Validate() error {
	if id := c.GetId(); len(id) == 0 {
		return errors.New("step id cannot be nil")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *StepConfig) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *StepConfig) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// UnmarshalJSON unmarshals json to the step config.
// For the config field: supports JSON, YAML, or a string containing either.
func (c *StepConfig) UnmarshalJSON(data []byte) error {
	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(jdata)
	if err != nil {
		return err
	}
	if v.Exists("id") {
		c.Id = string(v.GetStringBytes("id"))
	}
	if v.Exists("config") {
		var configVal *fastjson.Value
		configStr := v.GetStringBytes("config")
		if len(configStr) != 0 {
			// parse json and/or yaml
			configJson, err := yaml.YAMLToJSON(configStr)
			if err != nil {
				return err
			}
			var cj fastjson.Parser
			configVal, err = cj.ParseBytes(configJson)
			if err != nil {
				return err
			}
		} else {
			// expect a object value
			configVal = v.Get("config")
			if t := configVal.Type(); t != fastjson.TypeObject {
				return errors.Errorf("config: expected json object but got %s", t.String())
			}
		}
		// re-marshal to json
		c.Config = configVal.MarshalTo(nil)
	}
	return nil
}

// MarshalJSON marshals json from the step config.
// For the config field: supports JSON, YAML, or a string containing either.
func (c *StepConfig) MarshalJSON() ([]byte, error) {
	outCtr := gabs.New()

	// marshal the regular fields
	if configID := c.GetId(); configID != "" {
		_, err := outCtr.Set(configID, "id")
		if err != nil {
			return nil, err
		}
	}

	if confFieldData := c.GetConfig(); len(confFieldData) != 0 {
		// detect if the config field is json, if so, set it as inline json.
		if confFieldData[0] == '{' && confFieldData[len(confFieldData)-1] == '}' {
			confJSON, err := gabs.ParseJSON(confFieldData)
			if err != nil {
				return nil, err
			}
			_, err = outCtr.Set(confJSON, "config")
			if err != nil {
				return nil, err
			}
		} else {
			// otherwise encode it as base64 (this is what jsonpb does)
			_, err := outCtr.Set(base64.StdEncoding.EncodeToString(confFieldData), "config")
			if err != nil {
				return nil, err
			}
		}
	}

	// finalize the json
	return outCtr.EncodeJSON(), nil
}

// _ is a type assertion
var (
	_ block.Block      = ((*StepConfig)(nil))
	_ json.Unmarshaler = ((*StepConfig)(nil))
	_ json.Marshaler   = ((*StepConfig)(nil))
)
