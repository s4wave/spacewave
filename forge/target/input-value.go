package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// inputValue is the parsed and processed value of an Input.
type inputValue interface {
	// GetInputType returns the input type of this value.
	GetInputType() InputType
	// Validate checks the input value.
	Validate() error
	// IsEmpty reports whether the value is empty.
	IsEmpty() bool
}

// inputValueInline is the interface expected for a inputValue of type VALUE.
type inputValueInline interface {
	// inputValue indicates this is an inputValue.
	inputValue
	// GetValue returns the value.
	GetValue() *forge_value.Value
}

// InputValueWorld is the interface expected for a inputValue of type WORLD.
type InputValueWorld interface {
	// inputValue indicates this is an inputValue.
	inputValue
	// GetWorldEngineID returns the authoritative selected engine ID, or empty for an unscoped state.
	GetWorldEngineID() string
	// GetWorldEngine returns the world engine, if available.
	// May return nil if unavailable.
	GetWorldEngine() world.Engine
	// GetWorldState returns the world state.
	// Should not return nil.
	GetWorldState() world.WorldState
}

// inputValueWorldObject is the interface expected for a inputValue of type WORLD_OBJECT.
type inputValueWorldObject interface {
	// inputValue indicates this is an inputValue.
	inputValue
	// inputValueInline is the latest object state value.
	inputValueInline
	// InputValueWorld is the value for the world the object was retrieved from.
	InputValueWorld
}

// InputValueToValue resolves an inline inputValue to a Value.
// Returns nil, nil if the value is empty or nil.
func InputValueToValue(iv inputValue) (*forge_value.Value, error) {
	// Accept absent values and validate the resolved input type.
	if iv == nil {
		return nil, nil
	}
	inputType := iv.GetInputType()
	if err := inputType.validate(true); err != nil {
		return nil, err
	}

	// Convert value-bearing inputs through the inline value contract.
	switch inputType {
	case InputType_InputType_ALIAS:
		// unable to resolve alias with a value
		return nil, nil
	case InputType_InputType_VALUE:
		return inlineValueToValue(iv)
	case InputType_InputType_WORLD:
		return nil, errors.Wrap(ErrUnexpectedInputValueType, inputType.String())
	case InputType_InputType_WORLD_OBJECT, InputType_InputType_TASK_OUTPUT:
		return inlineValueToValue(iv)
	case InputType_InputType_UNKNOWN:
		return nil, nil
	default:
		return nil, errors.Wrap(ErrUnexpectedInputValueType, inputType.String())
	}
}

// InputValueToWorld resolves an inputValue to a InputValueWorld.
func InputValueToWorld(iv inputValue) (InputValueWorld, error) {
	// Accept an absent or empty World input.
	if iv == nil || iv.IsEmpty() {
		return nil, nil
	}

	// Require the resolved input to carry a World handle.
	vw, ok := iv.(InputValueWorld)
	if !ok {
		inputType := iv.GetInputType()
		if inputType != InputType_InputType_WORLD && inputType != InputType_InputType_WORLD_OBJECT {
			return nil, errors.Errorf("input type %s cannot be used as a world", inputType.String())
		}

		return nil, ErrUnexpectedInputValueType
	}

	return vw, nil
}

// InputValueToWorldState resolves an inputValue to a WorldState.
// Returns nil, nil if the value is empty or nil.
func InputValueToWorldState(iv inputValue) (world.WorldState, error) {
	// Resolve the World contract without discarding conversion errors.
	vw, err := InputValueToWorld(iv)
	if err != nil || vw == nil {
		return nil, err
	}

	// Validate the World input before returning its state.
	if err := iv.Validate(); err != nil {
		return nil, err
	}

	return vw.GetWorldState(), nil
}

// inlineValueToValue resolves an inline inputValue to a Value.
// Does not attempt to resolve dynamic values.
// Returns nil, nil if the value is empty or nil.
func inlineValueToValue(iv inputValue) (*forge_value.Value, error) {
	// Ignore absent inline inputs before converting their value.
	if iv == nil {
		return nil, nil
	}

	// Require the inline value contract before reading its payload.
	vw, ok := iv.(inputValueInline)
	if !ok {
		if iv.IsEmpty() {
			return nil, nil
		}
		inputType := iv.GetInputType()
		return nil, errors.Wrap(ErrUnexpectedInputValueType, inputType.String())
	}

	// Validate the inline payload before exposing it.
	if err := iv.Validate(); err != nil {
		return nil, err
	}

	return vw.GetValue(), nil
}
