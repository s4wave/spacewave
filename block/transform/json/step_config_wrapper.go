package block_transform_json

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block/transform"
)

// StepConfigWrapper implements the JSON unmarshaling and marshaling logic for a transform
// StepConfigWrapper.
type StepConfigWrapper struct {
	Id     string      `json:"id"`
	Config *StepConfig `json:"config"`
}

// NewStepConfigWrapper constructs a new step config wrapper.
func NewStepConfigWrapper(c config.Config) (*StepConfigWrapper, error) {
	return &StepConfigWrapper{
		Id: c.GetConfigID(),
		Config: &StepConfig{
			conf: c,
		},
	}, nil
}

// ResolveProto constructs the underlying config proto from the pending parse data.
func (c *StepConfigWrapper) ResolveProto(ts *block_transform.StepFactorySet) (*block_transform.StepConfig, error) {
	cc, err := c.Resolve(ts)
	if err != nil {
		return nil, err
	}
	return block_transform.NewStepConfig(cc)
}

// Resolve constructs the underlying config from the pending parse data.
func (c *StepConfigWrapper) Resolve(ts *block_transform.StepFactorySet) (config.Config, error) {
	configID := c.Id
	return c.Config.Resolve(ts, configID)
}
