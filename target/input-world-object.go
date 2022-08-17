package forge_target

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	forge_value "github.com/aperturerobotics/forge/value"
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
// note: if object_rev < input.object_rev, returns nil, nil, nil
// inpWorld and inpPrevValue can be nil
// may return nil, nil, nil if no value
func (i *InputWorldObject) ResolveValue(
	ctx context.Context,
	b bus.Bus,
	inpName string,
	inpWorld InputValueWorld,
) (InputValueWorldObject, func(), error) {
	// asserts objKey !=  ""
	err := i.Validate()
	if err != nil {
		return nil, nil, err
	}

	var inpObjs world.ObjectState
	var inpObjsExists bool
	var inpObjsValue InputValueInline
	objKey := i.GetObjectKey()
	if inpWorld != nil && !inpWorld.IsEmpty() {
		ws := inpWorld.GetWorldState()
		inpObjs, inpObjsExists, err = ws.GetObject(objKey)
		if err != nil {
			return nil, nil, err
		}
		if inpObjsExists {
			objSnapshot, err := forge_value.NewWorldObjectSnapshot(ctx, inpObjs, ws)
			if err != nil {
				return nil, nil, err
			}
			inpObjsValue = NewInputValueInline(forge_value.NewValueWithWorldObjectSnapshot(inpName, objSnapshot))
		} else {
			inpObjsValue = NewInputValueInline(forge_value.NewValue(inpName))
		}
	}

	var inpObjsRev uint64
	if inpObjsExists {
		_, inpObjsRev, err = inpObjs.GetRootRef()
		if err != nil {
			return nil, nil, err
		}
	}

	desiredMinimumRev := i.GetObjectRev()
	if desiredMinimumRev != 0 {
		if desiredMinimumRev > inpObjsRev {
			return nil, nil, nil
		}
	}

	return NewInputValueWorldObject(inpObjsValue, inpWorld, inpObjs, nil), nil, nil
}
