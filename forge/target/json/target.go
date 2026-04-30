package target_json

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/fastjson"
	"github.com/ghodss/yaml"
	target "github.com/s4wave/spacewave/forge/target"
)

// Target implements the JSON unmarshaling and marshaling logic for a Target.
// Resolve constructs the controller configs, looking up the types from the bus.
type Target struct {
	// underlying is a copy of the target without the "resolved" fields
	underlying *target.Target
	// execControllerConfig contains the json parser for the exec config
	execControllerConfig *configset_json.ControllerConfig
}

// UnmarshalYAML unmarshals YAML to a Target.
func UnmarshalYAML(data []byte) (*Target, error) {
	t := &Target{}
	if err := t.UnmarshalYAML(data); err != nil {
		return nil, err
	}
	return t, nil
}

// ResolveYAML parses the YAML target and resolves it on a bus.
func ResolveYAML(ctx context.Context, b bus.Bus, data []byte) (*target.Target, error) {
	tgtCtr, err := UnmarshalYAML(data)
	if err != nil {
		return nil, err
	}
	return tgtCtr.ResolveProto(ctx, b)
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
		o.Exec.Controller, err = configset_proto.NewControllerConfig(execConf, true)
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
	controllerVal := v.Get("exec", "controller")
	if controllerVal != nil && controllerVal.Type() == fastjson.TypeObject {
		cc, err := parseControllerConfigValue(controllerVal)
		if err != nil {
			return err
		}
		c.execControllerConfig = cc
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
	if c.underlying == nil {
		c.underlying = &target.Target{}
	} else {
		c.underlying.Reset()
	}
	if err := c.underlying.UnmarshalJSON(data); err != nil {
		return err
	}

	return nil
}

// MarshalJSON marshals a target JSON blob.
func (c *Target) MarshalJSON() ([]byte, error) {
	var p fastjson.Parser
	var arena fastjson.Arena
	v := arena.NewObject()

	// marshal the regular fields
	if c.underlying != nil {
		xdat, err := c.underlying.MarshalJSON()
		if err != nil {
			return nil, err
		}
		if len(xdat) != 0 {
			parsed, err := p.ParseBytes(xdat)
			if err != nil {
				return nil, err
			}
			v = arena.DeepCopyValue(parsed)
		}
	}

	// marshal the exec.controller config, if set
	if c.execControllerConfig != nil {
		cv, err := marshalControllerConfigValue(&arena, c.execControllerConfig)
		if err != nil {
			return nil, err
		}
		exec := v.Get("exec")
		if exec == nil || exec.Type() != fastjson.TypeObject {
			exec = arena.NewObject()
			v.Set("exec", exec)
		}
		exec.Set("controller", cv)
	}

	// finalize the json
	return v.MarshalTo(nil), nil
}
