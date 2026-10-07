package forge_target

import "github.com/pkg/errors"

// Validate validates the input type.
func (t InputType) Validate(allowUnknown bool) error {
	// Accept UNKNOWN only when the caller permits an empty input type.
	if t == InputType_InputType_UNKNOWN && allowUnknown {
		return nil
	}

	// Require a supported Target input type.
	switch t {
	case InputType_InputType_ALIAS:
	case InputType_InputType_VALUE:
	case InputType_InputType_WORLD:
	case InputType_InputType_WORLD_OBJECT:
	case InputType_InputType_TASK_OUTPUT:
	default:
		return errors.Wrap(ErrUnknownInputType, t.String())
	}
	return nil
}
