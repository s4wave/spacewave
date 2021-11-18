package forge_target

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/world"
)

// Validate validates the input world object.
func (i *InputWorldObject) Validate() error {
	if i.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ResolveValue resolves the InputWorldObject to a InputValueWorldObject.
//
// inpWorld and inpPrevValue can be nil
// may return nil, nil, nil if no value
func (i *InputWorldObject) ResolveValue(
	ctx context.Context,
	b bus.Bus,
	inpPrevValue InputValueInline,
	inpWorld InputValueWorld,
) (InputValueWorldObject, func(), error) {
	// asserts objKey !=  ""
	if err := i.Validate(); err != nil {
		return nil, nil, err
	}

	var inpObjs world.ObjectState
	objKey := i.GetObjectKey()
	if inpWorld != nil && !inpWorld.IsEmpty() {
		ws := inpWorld.GetWorldState()
		var err error
		inpObjs, _, err = ws.GetObject(objKey)
		if err != nil {
			return nil, nil, err
		}
	}

	return NewInputValueWorldObject(inpPrevValue, inpWorld, inpObjs, nil), nil, nil
}
