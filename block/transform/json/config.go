package block_transform_json

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/pkg/errors"
)

// Config implements the JSON unmarshaling and marshaling logic for a transform
// Config.
type Config struct {
	Steps []*StepConfigWrapper `json:"steps"`
}

// NewConfig constructs a new config with steps.
func NewConfig(steps []config.Config) (*Config, error) {
	oc := &Config{}
	for _, step := range steps {
		scw, err := NewStepConfigWrapper(step)
		if err != nil {
			return nil, err
		}
		oc.Steps = append(oc.Steps, scw)
	}
	return oc, nil
}

// ResolveProto constructs the underlying config from the pending parse data.
func (c *Config) ResolveProto(ts *block_transform.StepFactorySet) (*block_transform.Config, error) {
	o := &block_transform.Config{}
	for _, step := range c.Steps {
		p, err := step.ResolveProto(ts)
		if err != nil {
			return nil, err
		}
		o.Steps = append(o.Steps, p)
	}
	return o, nil
}

// Resolve constructs the underlying transforms from the pending parse data.
func (c *Config) Resolve(
	ts *block_transform.StepFactorySet,
	constructOpts controller.ConstructOpts,
) ([]block_transform.Step, []config.Config, error) {
	var confs []config.Config
	var steps []block_transform.Step
	for _, step := range c.Steps {
		cc, err := step.Resolve(ts)
		if err != nil {
			return nil, nil, err
		}
		f := ts.GetFactoryByConfigID(cc.GetConfigID())
		if f == nil {
			return nil, nil, errors.Errorf("transform not found: %s", cc.GetConfigID())
		}
		st, err := f.Construct(cc, constructOpts)
		if err != nil {
			return nil, nil, err
		}
		steps = append(steps, st)
		confs = append(confs, cc)
	}
	return steps, confs, nil
}
