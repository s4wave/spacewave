package forge_target

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// ResolveInputs resolves the Target's inputs and returns their release function
// and unresolved inputs. Input values supply aliases and scheduler-provided
// values. Other inputs resolve afresh so retained values cannot satisfy missing
// sources.
// ResolveTaskOutput supplies the Task package's completed-output reader.
func (t *Target) ResolveInputs(
	ctx context.Context,
	b bus.Bus,
	defWorld InputValueWorld,
	inputVals forge_value.ValueMap,
	resolveTaskOutput taskOutputResolver,
) (im InputMap, unresolved []*Input, relAll func(), err error) {
	// Reserve the resolved input map for the Target inputs.
	im = make(InputMap, len(t.GetInputs()))

	// Seed aliases and scheduler values, removing stale dynamically resolved inputs.
	for k, v := range inputVals {
		im[k] = NewInputValueInline(v)
	}
	for _, input := range t.GetInputs() {
		if input.GetInputType() != InputType_InputType_ALIAS {
			delete(im, input.GetName())
		}
	}

	// Chain input resource releases in their acquisition order.
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

	// Track Target input names resolved across dependent resolution passes.
	tgtInputs := t.GetInputs()
	resolved := make(map[string]struct{}, len(tgtInputs))

	// Resolve dependent Target inputs until a pass makes no further progress.
	prevResolved := -1
	for prevResolved != len(resolved) {
		prevResolved = len(resolved)
		var anyUnresolved bool
		for _, inp := range tgtInputs {
			inpName := inp.GetName()
			if _, ok := resolved[inpName]; ok {
				continue
			}
			inpVal, inpValRel, err := inp.resolveValue(ctx, b, im, defWorld, resolveTaskOutput)
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

	// Collect the unresolved Target inputs in name order.
	unresolved = make([]*Input, 0, len(tgtInputs)-len(resolved))
	for _, inp := range tgtInputs {
		// TODO: check Validate() here?
		inpVal := im[inp.GetName()]
		if inpVal == nil {
			unresolved = append(unresolved, inp)
		}
	}
	slices.SortFunc(unresolved, func(a, b *Input) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	return im, unresolved, relAll, nil
}

// resolveValue resolves one input against the inputs resolved so far, with an
// optional resource release function. Unavailable inputs return nil without
// error. ResolveTaskOutput supplies the Task package's completed-output reader.
func (i *Input) resolveValue(
	ctx context.Context,
	b bus.Bus,
	im InputMap,
	defWorld InputValueWorld,
	resolveTaskOutput taskOutputResolver,
) (inputValue, func(), error) {
	switch i.GetInputType() {
	case InputType_InputType_ALIAS:
		return im[i.GetName()], nil, nil
	case InputType_InputType_VALUE:
		return NewInputValueInline(i.GetValue()), nil, nil
	case InputType_InputType_WORLD:
		return i.GetWorld().ResolveValue(ctx, b)
	case InputType_InputType_TASK_OUTPUT:
		value, err := i.GetTaskOutput().resolveValue(ctx, i.GetName(), defWorld, resolveTaskOutput)
		return value, nil, err
	case InputType_InputType_WORLD_OBJECT:
		// Select the explicit World input or the default Forge Job World.
		inpWo := i.GetWorldObject()
		inpWorldID := inpWo.GetWorld()

		worldInp := defWorld
		if inpWorldID != "" {
			// A missing or mistyped World input resolves like an empty World.
			worldInp, _ = im[inpWorldID].(InputValueWorld)
		}

		// Resolve the object against the selected World input.
		return inpWo.resolveValue(ctx, b, i.GetName(), worldInp)
	case InputType_InputType_UNKNOWN:
		return nil, nil, nil
	}

	return nil, nil, errors.Wrap(ErrUnknownInputType, i.GetInputType().String())
}
