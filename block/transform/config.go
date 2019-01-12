package block_transform

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/golang/protobuf/proto"
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

// NewConfig constructs a new config with a set of underlying steps.
func NewConfig(steps []config.Config) (*Config, error) {
	c := &Config{}
	for _, step := range steps {
		sc, err := NewStepConfig(step)
		if err != nil {
			return nil, err
		}
		c.Steps = append(c.Steps, sc)
	}
	return c, nil
}
