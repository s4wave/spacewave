package target_json

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/Jeffail/gabs"
	"github.com/aperturerobotics/controllerbus/bus"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	target "github.com/aperturerobotics/forge/target"
	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"
	"github.com/valyala/fastjson"
	// "github.com/golang/protobuf/jsonpb"
)

// Target implements the JSON unmarshaling and marshaling logic for a Target.
// Resolve constructs the controller configs, looking up the types from the bus.
type Target struct {
	// underlying is a copy of the target without the "resolved" fields
	underlying *target.Target
	// execControllerConfig contains the json parser for the exec config
	execControllerConfig *configset_json.ControllerConfig
}

// UnmarshalYAML unmarshals the yaml to the target container.
func (c *Target) UnmarshalYAML(data []byte) error {
	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}
	return c.UnmarshalJSON(jdata)
}

// MarshalYAML marshals the container contents to yaml.
func (c *Target) MarshalYAML() ([]byte, error) {
	jdata, err := c.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jdata)
}

// SetTarget sets the target to marshal, resolving the controller config.
func (c *Target) SetTarget(ctx context.Context, b bus.Bus, pb *target.Target) error {
	c.underlying = &target.Target{
		Inputs:  pb.GetInputs(),
		Outputs: pb.GetOutputs(),
		Exec: &target.Exec{
			Disable: pb.GetExec().GetDisable(),
			// Controller: extracted below
		},
	}
	c.execControllerConfig = nil

	execController := pb.GetExec().GetController()
	if execController.GetId() != "" {
		// resolve the config object so we can marshal it
		conf, err := execController.Resolve(ctx, b)
		if err != nil {
			return err
		}
		c.execControllerConfig = configset_json.NewControllerConfig(conf)
	}

	return nil
}

// ResolveProto constructs the protobuf representation from the JSON parse data.
func (c *Target) ResolveProto(ctx context.Context, b bus.Bus) (*target.Target, error) {
	o := &target.Target{
		Inputs:  c.underlying.GetInputs(),
		Outputs: c.underlying.GetOutputs(),
		Exec: &target.Exec{
			Disable: c.underlying.GetExec().GetDisable(),
		},
	}
	// marshal the execControllerConfig to protobuf
	if c.execControllerConfig != nil {
		execConf, err := c.execControllerConfig.Resolve(ctx, b)
		if err != nil {
			return nil, err
		}
		o.Exec.Controller, err = configset_proto.NewControllerConfig(execConf)
		if err != nil {
			return nil, err
		}
	}
	return o, nil
}

// UnmarshalJSON unmarshals a target JSON blob pushing the data into the pending
// parse buffers.
func (c *Target) UnmarshalJSON(data []byte) error {
	// extract the exec.controller field
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return err
	}
	if c.underlying == nil {
		c.underlying = &target.Target{}
	}
	var changed bool
	obj := v.GetObject("exec", "controller")
	if obj != nil {
		// re-marshal just that portion of the object
		execControllerData := obj.MarshalTo(nil)
		// push it into the parser
		c.execControllerConfig = &configset_json.ControllerConfig{}
		if err := json.Unmarshal(execControllerData, c.execControllerConfig); err != nil {
			return err
		}
		c.underlying.Exec = &target.Exec{
			Disable: v.GetBool("exec", "disable"),
		}
		// delete it from the pending parse
		v.Del("exec")
		changed = true
	}

	if changed {
		data = v.MarshalTo(nil)
	}

	// use jsonpb to parse everything else
	if err := jsonpb.UnmarshalString(string(data), c.underlying); err != nil {
		return err
	}

	return nil
}

// MarshalJSON marshals a target JSON blob.
func (c *Target) MarshalJSON() ([]byte, error) {
	m := &jsonpb.Marshaler{}
	var v *gabs.Container

	// marshal the regular fields
	if c.underlying != nil {
		var b bytes.Buffer
		err := m.Marshal(&b, c.underlying)
		if err != nil {
			return nil, err
		}
		// parse the json to gabs format
		gj, err := gabs.ParseJSONBuffer(&b)
		if err != nil {
			return nil, err
		}
		v = gj
	}

	if v == nil {
		var err error
		v, err = gabs.New().Object()
		if err != nil {
			return nil, err
		}
	}

	// marshal the exec.controller config, if set
	if c.execControllerConfig != nil {
		dat, err := json.Marshal(c.execControllerConfig)
		if err != nil {
			return nil, err
		}
		// parse the json to gabs format
		gv, err := gabs.ParseJSON(dat)
		if err != nil {
			return nil, err
		}
		// set the field
		_, err = v.Set(gv, "exec", "controller")
		if err != nil {
			return nil, err
		}
	}

	// finalize the json
	return v.EncodeJSON(), nil
}

// _ is a type assertion
var _ json.Unmarshaler = ((*Target)(nil))

// _ is a type assertion
var _ json.Marshaler = ((*Target)(nil))
