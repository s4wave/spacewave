package sobject

import (
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
)

// RedactStepConfig returns a copy of the StepConfig with sensitive fields
// (encryption keys) zeroed. The algorithm and other non-sensitive fields
// are preserved. Returns the original step unchanged for non-blockenc steps.
func RedactStepConfig(step *block_transform.StepConfig) *block_transform.StepConfig {
	if step == nil {
		return nil
	}
	out := step.CloneVT()
	if out.GetId() != blockenc.ConfigID {
		return out
	}
	data := out.GetConfig()
	if len(data) == 0 {
		return out
	}
	conf := &blockenc.Config{}
	// config bytes can be proto or JSON (first byte '{')
	if data[0] == '{' {
		if err := conf.UnmarshalJSON(data); err != nil {
			return out
		}
	} else {
		if err := conf.UnmarshalVT(data); err != nil {
			return out
		}
	}
	conf.Key = nil
	redacted, err := conf.MarshalVT()
	if err != nil {
		return out
	}
	out.Config = redacted
	return out
}

// RedactStepConfigs returns redacted copies of all step configs.
func RedactStepConfigs(steps []*block_transform.StepConfig) []*block_transform.StepConfig {
	out := make([]*block_transform.StepConfig, len(steps))
	for i, step := range steps {
		out[i] = RedactStepConfig(step)
	}
	return out
}
