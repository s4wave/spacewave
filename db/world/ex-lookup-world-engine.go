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
	returnIfIdle bool,
	id string,
	disposeCb func(),
) (LookupWorldEngineValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupWorldEngineValue](
		ctx,
		b,
		NewLookupWorldEngine(id),
		bus.ReturnIfIdle(returnIfIdle),
		disposeCb,
		nil,
	)
}
