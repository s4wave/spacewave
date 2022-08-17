package forge_target

import (
	"context"
	"sort"

	"github.com/aperturerobotics/controllerbus/bus"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/pkg/errors"
)

// ResolveInputMap resolves a ValueMap to an InputMap.
// returns a function which can be used to release the values.
// inputVals is the contents of the inputs ValueSet.
// returns the list of unresolved inputs.
func ResolveInputMap(
	ctx context.Context,
	b bus.Bus,
	defWorld InputValueWorld,
	tgt *Target,
	inputVals forge_value.ValueMap,
) (im InputMap, unresolved []*Input, relAll func(), err error) {
	im = make(InputMap, len(tgt.GetInputs()))

	// add all values provided in the value map
	for k, v := range inputVals {
		im[k] = NewInputValueInline(v)
	}

	// resolve all Input from the Target.
	appendRel := func(rel func()) {
		if rel != nil {
			oldRel := relAll
			relAll = func() {
				if oldRel != nil {
					oldRel()
				}
				rel()
			}
		}
	}

	// may require multiple passes to resolve all inputs.
	tgtInputs := tgt.GetInputs()
	resolved := make(map[string]struct{}, len(tgtInputs))

	prevResolved := -1
	for {
		if prevResolved == len(resolved) {
			// no progress
			break
		}
		prevResolved = len(resolved)
		var anyUnresolved bool
		for _, inp := range tgtInputs {
			inpName := inp.GetName()
			if _, ok := resolved[inpName]; ok {
				continue
			}
			inpVal, inpValRel, err := ResolveInput(ctx, b, inp, im, defWorld)
			if err != nil {
				if relAll != nil {
					relAll()
				}
				return nil, nil, nil, err
			}
			if inpVal == nil {
				anyUnresolved = true
				continue
			}

			appendRel(inpValRel)
			im[inpName] = inpVal
			resolved[inpName] = struct{}{}
		}
		if !anyUnresolved {
			break
		}
	}
	if relAll == nil {
		relAll = func() {}
	}

	unresolved = make([]*Input, 0, len(tgtInputs)-len(resolved))
	for _, inp := range tgtInputs {
		// TODO: check Validate() here?
		inpVal := im[inp.GetName()]
		if inpVal == nil {
			unresolved = append(unresolved, inp)
		}
	}
	sort.Slice(unresolved, func(i, j int) bool {
		return unresolved[i].GetName() < unresolved[j].GetName()
	})

	return im, unresolved, relAll, nil
}

// ResolveInput resolves an input to the InputMap.
// can return an optional release func (or nil)
// can return nil, nil, nil if no value
func ResolveInput(
	ctx context.Context,
	b bus.Bus,
	inp *Input,
	im InputMap,
	defWorld InputValueWorld,
) (InputValue, func(), error) {
	switch inp.GetInputType() {
	case InputType_InputType_ALIAS:
		return im[inp.GetName()], nil, nil
	case InputType_InputType_VALUE:
		return NewInputValueInline(inp.GetValue()), nil, nil
	case InputType_InputType_WORLD:
		return inp.GetWorld().ResolveValue(ctx, b)
	case InputType_InputType_WORLD_OBJECT:
		// get the world input first
		inpWo := inp.GetWorldObject()
		inpWorldID := inpWo.GetWorld()

		var worldInp InputValueWorld
		if inpWorldID == "" {
			// use the default forge world as the world input
			// may be nil
			worldInp = defWorld
		} else {
			// lookup the world input
			worldInpVal, worldInpValOk := im[inpWorldID]
			if worldInpValOk && !worldInpVal.IsEmpty() {
				// note: could check if !ok here, but instead:
				// if the type doesn't match, treat it like an empty value.
				worldInp, _ = worldInpVal.(InputValueWorld)
			}
		}

		// resolve the world object
		return inpWo.ResolveValue(ctx, b, inp.GetName(), worldInp)
	case InputType_InputType_UNKNOWN:
		return nil, nil, nil
	}

	return nil, nil, errors.Wrap(ErrUnknownInputType, inp.GetInputType().String())
}
