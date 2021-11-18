package forge_target

import "github.com/pkg/errors"

// Validate validates the input type.
func (t InputType) Validate(allowUnknown bool) error {
	if t == InputType_InputType_UNKNOWN {
		if allowUnknown {
			return nil
		}
	}
	switch t {
	case InputType_InputType_ALIAS:
	case InputType_InputType_VALUE:
	case InputType_InputType_WORLD:
	case InputType_InputType_WORLD_OBJECT:
	default:
		return errors.Wrap(ErrUnknownInputType, t.String())
	}
	return nil
}
