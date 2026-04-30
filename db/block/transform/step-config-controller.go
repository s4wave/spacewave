//go:build !sql_lite

package block_transform

import "github.com/aperturerobotics/controllerbus/config"

// NewStepConfig constructs the step config with a underlying config.
func NewStepConfig(conf config.Config) (*StepConfig, error) {
	dat, err := conf.MarshalVT()
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
		return conf.UnmarshalJSON(data)
	}
	return conf.UnmarshalVT(data)
}
