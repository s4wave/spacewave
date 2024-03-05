package web_runtime

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupWebRuntime is a directive to lookup a WebRuntime.
type LookupWebRuntime interface {
	// Directive indicates LookupWebRuntime is a directive.
	directive.Directive

	// LookupWebRuntimeID is the web runtime ID to lookup.
	// Cannot be empty.
	LookupWebRuntimeID() string
}

// LookupWebRuntimeValue is the result of LookupWebRuntime.
type LookupWebRuntimeValue = WebRuntime

// lookupWebRuntime implements LookupWebRuntime
type lookupWebRuntime struct {
	webRuntimeID string
}

// NewLookupWebRuntime constructs a new LookupWebRuntime directive.
func NewLookupWebRuntime(webRuntimeID string) LookupWebRuntime {
	return &lookupWebRuntime{webRuntimeID: webRuntimeID}
}

// ExLookupWebRuntime looks up a web view by id.
func ExLookupWebRuntime(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	webRuntimeID string,
) (LookupWebRuntimeValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupWebRuntimeValue](
		ctx,
		b,
		NewLookupWebRuntime(webRuntimeID),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupWebRuntime) Validate() error {
	if d.webRuntimeID == "" {
		return ErrEmptyWebRuntimeID
	}
	return nil
}

// GetValueLookupWebRuntimeOptions returns options relating to value handling.
func (d *lookupWebRuntime) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupWebRuntime is the web view ID to lookup.
func (d *lookupWebRuntime) LookupWebRuntimeID() string {
	return d.webRuntimeID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupWebRuntime) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupWebRuntime)
	if !ok {
		return false
	}

	if d.LookupWebRuntimeID() != od.LookupWebRuntimeID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupWebRuntime) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWebRuntime) GetName() string {
	return "LookupWebRuntime"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupWebRuntime) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["web-runtime-id"] = []string{d.LookupWebRuntimeID()}
	return vals
}

// _ is a type assertion
var _ LookupWebRuntime = ((*lookupWebRuntime)(nil))
