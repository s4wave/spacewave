package session

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupSessionController is a directive to look up a session controller.
type LookupSessionController interface {
	// Directive indicates LookupSessionController is a directive.
	directive.Directive

	// LookupSessionControllerID returns the session controller id to lookup.
	// Empty to return all.
	LookupSessionControllerID() string
}

// LookupSessionControllerValue is the result type for LookupSessionController.
type LookupSessionControllerValue = SessionController

// ExLookupSessionController executes a lookup for a single session controller on the bus.
//
// id should be set to filter to a specific session controller id
// If waitOne is set, waits for at least one value before returning.
// Returns when the directive becomes idle.
func ExLookupSessionController(
	ctx context.Context,
	b bus.Bus,
	id string,
	returnIfIdle bool,
	valDisposeCb func(),
) (SessionController, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[LookupSessionControllerValue](ctx, b, NewLookupSessionController(id), bus.ReturnIfIdle(returnIfIdle), valDisposeCb)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		avRef.Release()
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// lookupSessionController implements LookupSessionController
type lookupSessionController struct {
	id string
}

// NewLookupSessionController constructs a new LookupSessionController directive.
func NewLookupSessionController(id string) LookupSessionController {
	return &lookupSessionController{
		id: id,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupSessionController) Validate() error {
	return nil
}

// GetValueLookupProviderOptions returns options relating to value handling.
func (d *lookupSessionController) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupSessionControllerID returns the id to lookup.
func (d *lookupSessionController) LookupSessionControllerID() string {
	return d.id
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupSessionController) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupSessionController)
	if !ok {
		return false
	}

	if d.LookupSessionControllerID() != od.LookupSessionControllerID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupSessionController) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupSessionController) GetName() string {
	return "LookupSessionController"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupSessionController) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.id != "" {
		vals["id"] = []string{d.id}
	}
	return vals
}

// _ is a type assertion
var _ LookupSessionController = ((*lookupSessionController)(nil))
