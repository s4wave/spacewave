package provider

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupProviderInfo is a directive to look up provider info.
// Usually used to list the available providers.
type LookupProviderInfo interface {
	// Directive indicates LookupProviderInfo is a directive.
	directive.Directive

	// LookupProviderInfoID returns the provider id to lookup.
	// Empty to look up all.
	LookupProviderInfoID() string
}

// LookupProviderInfoValue is the result type for LookupProviderInfo.
type LookupProviderInfoValue = *ProviderInfo

// ExLookupProviderInfos executes a lookup for all of the providers on the bus.
//
// id can optionally filter to a specific provider id.
// If waitOne is set, waits for at least one value before returning.
// Returns when the directive becomes idle.
func ExLookupProviderInfos(
	ctx context.Context,
	b bus.Bus,
	id string,
	waitOne bool,
) ([]LookupProviderInfoValue, error) {
	dir := NewLookupProviderInfo(id)
	sess, _, dirRef, err := bus.ExecCollectValues[LookupProviderInfoValue](ctx, b, dir, waitOne, nil)
	if err != nil {
		return nil, err
	}
	if dirRef != nil {
		dirRef.Release()
	}
	return sess, nil
}

// lookupProviderInfo implements LookupProviderInfo
type lookupProviderInfo struct {
	id string
}

// NewLookupProviderInfo constructs a new LookupProviderInfo directive.
func NewLookupProviderInfo(id string) LookupProviderInfo {
	return &lookupProviderInfo{
		id: id,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupProviderInfo) Validate() error {
	return nil
}

// GetValueLookupProviderInfoOptions returns options relating to value handling.
func (d *lookupProviderInfo) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupProviderInfoID returns the id to lookup.
func (d *lookupProviderInfo) LookupProviderInfoID() string {
	return d.id
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupProviderInfo) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupProviderInfo)
	if !ok {
		return false
	}

	if d.LookupProviderInfoID() != od.LookupProviderInfoID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupProviderInfo) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupProviderInfo) GetName() string {
	return "LookupProviderInfo"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupProviderInfo) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.id != "" {
		vals["id"] = []string{d.id}
	}
	return vals
}

// _ is a type assertion
var _ LookupProviderInfo = ((*lookupProviderInfo)(nil))
