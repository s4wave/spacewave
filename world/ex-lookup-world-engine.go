package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// ExLookupWorldEngine executes a single-value engine lookup against a bus by ID.
// If ID is empty, selects any.
func ExLookupWorldEngine(
	ctx context.Context,
	b bus.Bus,
	id string,
) (LookupWorldEngineValue, directive.Reference, error) {
	v, ref, err := bus.ExecOneOff(ctx, b, NewLookupWorldEngine(id), nil)
	if err != nil {
		return nil, nil, err
	}
	return v.GetValue().(LookupWorldEngineValue), ref, nil
}
