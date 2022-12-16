package block_transform_json

import (
	"encoding/json"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/pkg/errors"
)

// StepConfig implements the JSON unmarshaling and marshaling logic for a transform
// StepConfig.
type StepConfig struct {
	pendingParseData []byte
	conf             config.Config
}

// SetConfig sets the underlying config.
func (c *StepConfig) SetConfig(cc config.Config) {
	c.conf = cc
}

// Resolve constructs the underlying config from the pending parse data.
func (c *StepConfig) Resolve(
	ts *block_transform.StepFactorySet,
	configID string,
) (config.Config, error) {
	tf := ts.GetStepFactoryByConfigID(configID)
	if tf == nil {
		return nil, errors.Errorf("unknown transform: %s", configID)
	}
	cc := tf.ConstructConfig()
	if err := json.Unmarshal(c.pendingParseData, cc); err != nil {
		return nil, err
	}
	return cc, nil
}

// UnmarshalJSON unmarshals a controller config JSON blob pushing the data into
// the pending parse buffer.
func (c *StepConfig) UnmarshalJSON(data []byte) error {
	c.pendingParseData = make([]byte, len(data))
	copy(c.pendingParseData, data)
	return nil
}

// MarshalJSON marshals a controller config JSON blob.
func (c *StepConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.conf)
}

// _ is a type assertion
var _ json.Unmarshaler = ((*StepConfig)(nil))

// _ is a type assertion
var _ json.Marshaler = ((*StepConfig)(nil))
