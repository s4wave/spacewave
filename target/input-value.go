package forge_target

import (
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// InputValue is the parsed and processed value of an Input.
type InputValue interface {
	// GetInputType returns the input type of this value.
	GetInputType() InputType
	// Validate checks the input value.
	Validate() error
	// IsEmpty checks if the value is "empty."
	IsEmpty() bool
}

// InputValueInline is the interface expected for a InputValue of type VALUE.
type InputValueInline interface {
	// InputValue indicates this is an InputValue.
	InputValue
	// GetValue returns the value.
	GetValue() *forge_value.Value
}

// InputValueWorld is the interface expected for a InputValue of type WORLD.
type InputValueWorld interface {
	// InputValue indicates this is an InputValue.
	InputValue
	// GetWorldEngine returns the world engine, if available.
	// May return nil if unavailable.
	GetWorldEngine() world.Engine
	// GetWorldState returns the world state.
	// Should not return nil.
	GetWorldState() world.WorldState
}

// InputValueWorldObject is the interface expected for a InputValue of type WORLD_OBJECT.
type InputValueWorldObject interface {
	// InputValue indicates this is an InputValue.
	InputValue
	// InputValueInline is the latest object state value.
	InputValueInline
	// InputValueWorld is the value for the world the object was retrieved from.
	InputValueWorld
	// GetWorldObject returns the world object state handle.
	GetWorldObject() world.ObjectState
}

// InputValueToWorldState resolves an InputValue to a WorldState.
// Returns nil, nil if the value is empty or nil.
func InputValueToWorldState(iv InputValue) (world.WorldState, error) {
	if iv == nil || iv.IsEmpty() {
		return nil, nil
	}

	inputType := iv.GetInputType()
	if inputType != InputType_InputType_WORLD {
		return nil, errors.Errorf("input type %s cannot be used as a world", inputType.String())
	}

	vw, ok := iv.(InputValueWorld)
	if !ok {
		return nil, ErrUnexpectedInputValueType
	}

	if err := iv.Validate(); err != nil {
		return nil, err
	}

	return vw.GetWorldState(), nil
}

// InputValueToValue resolves an inline InputValue to a Value.
// Resolves dynamic values: if WORLD_OBJECT, looks up the object, etc.
// Returns nil, nil if the value is empty or nil.
func InputValueToValue(iv InputValue) (*forge_value.Value, error) {
	if iv == nil {
		return nil, nil
	}
	inputType := iv.GetInputType()
	if err := inputType.Validate(true); err != nil {
		return nil, err
	}

	switch inputType {
	case InputType_InputType_ALIAS:
		// unable to resolve alias with a value
		return nil, nil
	case InputType_InputType_VALUE:
		return InlineValueToValue(iv)
	case InputType_InputType_WORLD:
		return nil, errors.Wrap(ErrUnexpectedInputValueType, inputType.String())
	case InputType_InputType_WORLD_OBJECT:
		return WorldObjectToValue(iv, false)
	case InputType_InputType_UNKNOWN:
		return nil, nil
	default:
		return nil, errors.Wrap(ErrUnexpectedInputValueType, inputType.String())
	}
}

// InlineValueToValue resolves an inline InputValue to a Value.
// Does not attempt to resolve dynamic values.
// Returns nil, nil if the value is empty or nil.
func InlineValueToValue(iv InputValue) (*forge_value.Value, error) {
	if iv == nil {
		return nil, nil
	}

	vw, ok := iv.(InputValueInline)
	if !ok {
		if vw.IsEmpty() {
			return nil, nil
		}
		inputType := iv.GetInputType()
		return nil, errors.Wrap(ErrUnexpectedInputValueType, inputType.String())
	}

	if err := iv.Validate(); err != nil {
		return nil, err
	}

	return vw.GetValue(), nil
}

// WorldObjectToValue resolves a WorldObject to a value.
//
// if forceLookup is set, disallows using the InputValueInline as a fallback.
// returns nil, nil if empty or object not found
func WorldObjectToValue(iv InputValue, forceLookup bool) (*forge_value.Value, error) {
	if iv == nil {
		return nil, nil
	}

	var wobj world.ObjectState
	wv, ok := iv.(InputValueWorldObject)
	if ok {
		wobj = wv.GetWorldObject()
	}
	if wobj != nil {
		wobjRoot, _, err := wobj.GetRootRef()
		if err != nil {
			return nil, err
		}
		return forge_value.NewValueWithBucketRef(wobj.GetKey(), wobjRoot), nil
	}
	if forceLookup {
		return nil, nil
	}

	inv, ok := iv.(InputValueInline)
	if ok {
		return inv.GetValue(), nil
	}
	return nil, nil
}
