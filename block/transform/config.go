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
