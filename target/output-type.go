package forge_target

import "github.com/pkg/errors"

// Validate validates the output type.
func (t OutputType) Validate(allowUnknown bool) error {
	if t == OutputType_OutputType_UNKNOWN {
		if allowUnknown {
			return nil
		}
	}
	switch t {
	case OutputType_OutputType_EXEC:
	case OutputType_OutputType_VALUE:
	default:
		return errors.Wrap(ErrUnknownOutputType, t.String())
	}
	return nil
}
