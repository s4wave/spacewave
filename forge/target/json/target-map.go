package target_json

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/fastjson"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	target "github.com/s4wave/spacewave/forge/target"
)

// TargetMap implements the JSON unmarshaling and marshaling logic for a map of
// targets. Resolve constructs the controller configs, looking up the types.
type TargetMap struct {
	underlying map[string]*Target
}

// UnmarshalYAML unmarshals YAML to a TargetMap.
func UnmarshalTargetMapYAML(data []byte) (*TargetMap, error) {
	t := &TargetMap{}
	if err := t.UnmarshalYAML(data); err != nil {
		return nil, err
	}
	return t, nil
}

// ResolveYAML parses the YAML target map and resolves it on a bus.
func ResolveTargetMapYAML(ctx context.Context, b bus.Bus, data []byte) (map[string]*target.Target, error) {
	tgtCtr, err := UnmarshalTargetMapYAML(data)
	if err != nil {
		return nil, err
	}
	return tgtCtr.ResolveProto(ctx, b)
}

// UnmarshalYAML unmarshals the yaml to the target container.
func (c *TargetMap) UnmarshalYAML(data []byte) error {
	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}
	return c.UnmarshalJSON(jdata)
}

// MarshalYAML marshals the container contents to yaml.
func (c *TargetMap) MarshalYAML() ([]byte, error) {
	jdata, err := c.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jdata)
}

// SetTargetMap sets the target to marshal, resolving the controller config.
func (c *TargetMap) SetTargetMap(ctx context.Context, b bus.Bus, pb map[string]*target.Target) error {
	c.underlying = make(map[string]*Target)
	for k, v := range pb {
		tgt := &Target{}
		if err := tgt.SetTarget(ctx, b, v); err != nil {
			return errors.Wrap(err, k)
		}
		c.underlying[k] = tgt
	}
	return nil
}

// ResolveProto constructs the protobuf representation from the JSON parse data.
func (c *TargetMap) ResolveProto(ctx context.Context, b bus.Bus) (map[string]*target.Target, error) {
	o := make(map[string]*target.Target, len(c.underlying))
	var err error
	for k, v := range c.underlying {
		o[k], err = v.ResolveProto(ctx, b)
		if err != nil {
			return nil, errors.Wrap(err, k)
		}
	}
	return o, nil
}

// UnmarshalJSON unmarshals a target JSON blob pushing the data into the pending
// parse buffers.
func (c *TargetMap) UnmarshalJSON(data []byte) error {
	c.underlying = make(map[string]*Target)

	// parse the json to a container
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return err
	}
	obj, err := v.Object()
	if err != nil {
		return err
	}
	obj.Visit(func(key []byte, v *fastjson.Value) {
		if err != nil {
			return
		}
		tgt := &Target{}
		err = tgt.UnmarshalJSON(v.MarshalTo(nil))
		if err != nil {
			err = errors.Wrap(err, string(key))
		} else {
			c.underlying[string(key)] = tgt
		}
	})
	return err
}

// MarshalJSON marshals a target JSON blob.
func (c *TargetMap) MarshalJSON() ([]byte, error) {
	var p fastjson.Parser
	obj := &fastjson.Object{}
	for k, v := range c.underlying {
		jd, err := v.MarshalJSON()
		if err != nil {
			return nil, err
		}
		jv, err := p.ParseBytes(jd)
		if err != nil {
			return nil, err
		}
		obj.Set(k, jv)
	}
	return obj.MarshalTo(nil), nil
}
