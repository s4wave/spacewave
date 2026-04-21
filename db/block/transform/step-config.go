package block_transform

import (
	"encoding/base64"

	"github.com/Jeffail/gabs/v2"
	"github.com/aperturerobotics/controllerbus/config"
	jsoniter "github.com/aperturerobotics/json-iterator-lite"
	"github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

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

// MarshalProtoJSON marshals the ControllerConfig message to JSON.
func (c *StepConfig) MarshalProtoJSON(s *json.MarshalState) {
	if c == nil {
		s.WriteNil()
		return
	}
	s.WriteObjectStart()
	var wroteField bool
	if c.Id != "" || s.HasField("id") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("id")
		s.WriteString(c.Id)
	}
	if len(c.Config) > 0 || s.HasField("config") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("config")
		// Detect if config is JSON
		if c.Config[0] == '{' && c.Config[len(c.Config)-1] == '}' {
			// Ensure json is parseable
			_, err := gabs.ParseJSON(c.Config)
			if err != nil {
				s.SetError(errors.Wrap(err, "unable to parse config json"))
				return
			}
			_, err = s.Write(c.Config)
			if err != nil {
				s.SetError(err)
				return
			}
		} else {
			// Base58 encoded string
			s.WriteString(base64.RawStdEncoding.EncodeToString(c.Config))
		}
	}
	s.WriteObjectEnd()
}

// MarshalJSON marshals the ControllerConfig to JSON.
func (c *StepConfig) MarshalJSON() ([]byte, error) {
	return json.DefaultMarshalerConfig.Marshal(c)
}

// UnmarshalJSON unmarshals the ControllerConfig from JSON.
func (c *StepConfig) UnmarshalJSON(b []byte) error {
	return json.DefaultUnmarshalerConfig.Unmarshal(b, c)
}

// UnmarshalProtoJSON unmarshals the StepConfig from a ProtoJSON state.
func (c *StepConfig) UnmarshalProtoJSON(s *json.UnmarshalState) {
	for key := s.ReadObjectField(); key != ""; key = s.ReadObjectField() {
		switch key {
		case "id":
			c.Id = s.ReadString()
		case "config":
			if s.ReadNil() {
				break
			}
			switch s.WhatIsNext() {
			case jsoniter.StringValue:
				var err error
				c.Config, err = base64.RawStdEncoding.DecodeString(s.ReadString())
				if err != nil {
					s.SetError(errors.Wrap(err, "unmarshal config value as base58 string"))
					return
				}
			case jsoniter.ObjectValue:
				c.Config = s.SkipAndReturnBytes()
			default:
				s.SetError(errors.New("invalid json value for config"))
				return
			}
		default:
			s.Skip()
		}
	}
}

// _ is a type assertion
var (
	_ block.Block      = ((*StepConfig)(nil))
	_ json.Unmarshaler = ((*StepConfig)(nil))
	_ json.Marshaler   = ((*StepConfig)(nil))
)
