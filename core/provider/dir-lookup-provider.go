package provider

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupProvider is a directive to look up a provider.
type LookupProvider interface {
	// Directive indicates LookupProvider is a directive.
	directive.Directive

	// LookupProviderID returns the provider id to lookup.
	LookupProviderID() string
}

// LookupProviderValue is the result type for LookupProvider.
type LookupProviderValue = Provider

// ExLookupProvider executes a lookup for a single provider on the bus.
//
// id should be set to filter to a specific provider id
// If waitOne is set, waits for at least one value before returning.
// Returns when the directive becomes idle.
func ExLookupProvider(
	ctx context.Context,
	b bus.Bus,
	id string,
	returnIfIdle bool,
	valDisposeCb func(),
) (Provider, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[LookupProviderValue](ctx, b, NewLookupProvider(id), bus.ReturnIfIdle(returnIfIdle), valDisposeCb)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		avRef.Release()
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// ExLookupProviders executes a lookup for all of the providers on the bus.
//
// id can optionally filter to a specific provider id.
// If waitOne is set, waits for at least one value before returning.
// Returns when the directive becomes idle.
func ExLookupProviders(
	ctx context.Context,
	b bus.Bus,
	id string,
	waitOne bool,
) ([]LookupProviderValue, directive.Instance, directive.Reference, error) {
	dir := NewLookupProvider(id)
	return bus.ExecCollectValues[LookupProviderValue](ctx, b, dir, waitOne, nil)
}

// lookupProvider implements LookupProvider
type lookupProvider struct {
	id string
}

// NewLookupProvider constructs a new LookupProvider directive.
func NewLookupProvider(id string) LookupProvider {
	return &lookupProvider{
		id: id,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupProvider) Validate() error {
	return nil
}

// GetValueLookupProviderOptions returns options relating to value handling.
func (d *lookupProvider) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupProviderID returns the id to lookup.
func (d *lookupProvider) LookupProviderID() string {
	return d.id
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupProvider) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupProvider)
	if !ok {
		return false
	}

	if d.LookupProviderID() != od.LookupProviderID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupProvider) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupProvider) GetName() string {
	return "LookupProvider"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupProvider) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.id != "" {
		vals["id"] = []string{d.id}
	}
	return vals
}

// _ is a type assertion
var _ LookupProvider = ((*lookupProvider)(nil))
