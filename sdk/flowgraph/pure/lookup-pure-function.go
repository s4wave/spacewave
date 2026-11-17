package flowgraph_pure

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupPureFunction is a directive to find the component that evaluates a
// pure function.
type LookupPureFunction interface {
	// Directive indicates LookupPureFunction is a directive.
	directive.Directive

	// LookupPureFunctionID returns the function ID to look up.
	LookupPureFunctionID() string
}

// LookupPureFunctionValue is the result type for LookupPureFunction.
type LookupPureFunctionValue = Function

// ExLookupPureFunction waits until a component registers the function and
// returns it. It blocks while no component serves the function, so a pure
// Step whose function is not loaded waits here. Release the returned
// reference when done with the function.
func ExLookupPureFunction(ctx context.Context, b bus.Bus, functionID string) (Function, directive.Reference, error) {
	av, _, ref, err := bus.ExecOneOffTyped[LookupPureFunctionValue](ctx, b, NewLookupPureFunction(functionID), nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return av.GetValue(), ref, nil
}

// lookupPureFunction implements LookupPureFunction.
type lookupPureFunction struct {
	// functionID is the function to look up.
	functionID string
}

// NewLookupPureFunction constructs a LookupPureFunction directive.
func NewLookupPureFunction(functionID string) LookupPureFunction {
	return &lookupPureFunction{functionID: functionID}
}

// Validate requires a function ID.
func (d *lookupPureFunction) Validate() error {
	if d.functionID == "" {
		return ErrEmptyFunctionID
	}
	return nil
}

// GetValueOptions disposes the lookup shortly after its last reference.
func (d *lookupPureFunction) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupPureFunctionID returns the function ID to look up.
func (d *lookupPureFunction) LookupPureFunctionID() string {
	return d.functionID
}

// IsEquivalent reports whether the other directive looks up the same function.
func (d *lookupPureFunction) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupPureFunction)
	return ok && od.LookupPureFunctionID() == d.functionID
}

// Superceeds returns false: lookups never cancel each other.
func (d *lookupPureFunction) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *lookupPureFunction) GetName() string {
	return "LookupPureFunction"
}

// GetDebugVals returns the function ID.
func (d *lookupPureFunction) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"function-id": []string{d.functionID}}
}

// _ is a type assertion
var _ LookupPureFunction = (*lookupPureFunction)(nil)
