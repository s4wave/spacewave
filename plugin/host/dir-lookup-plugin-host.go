package plugin_host

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupPluginHost is a directive to look up available plugin hosts on the bus.
type LookupPluginHost interface {
	// Directive indicates LookupPluginHost is a directive.
	directive.Directive

	// LookupPluginHostPlatformIDs filters the platform IDs to match.
	// If unset (empty), matches all platform IDs.
	LookupPluginHostPlatformIDs() []string
}

// LookupPluginHostValue is the result type for LookupPluginHost.
// Multiple results may be pushed to the directive.
type LookupPluginHostValue = PluginHost

// lLookupPluginHost implements LookupPluginHost
type lLookupPluginHost struct {
	platformIDs []string
}

// NewLookupPluginHost constructs a new LookupPluginHost directive.
func NewLookupPluginHost(platformIDs []string) LookupPluginHost {
	if len(platformIDs) != 0 {
		platformIDs = slices.Clone(platformIDs)
		slices.Sort(platformIDs)
		platformIDs = slices.Compact(platformIDs)
	}
	return &lLookupPluginHost{platformIDs: platformIDs}
}

// ExLookupPluginHostByPlatform executes the LookupPluginHost directive for a single platform ID.
//
// if returnIfIdle=true and the directive becomes idle, returns nil, nil, nil
func ExLookupPluginHostByPlatform(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	platformID string,
	valDisposeCallback func(),
) (LookupPluginHostValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupPluginHostValue](
		ctx,
		b,
		NewLookupPluginHost([]string{platformID}),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCallback,
		nil,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lLookupPluginHost) Validate() error {
	return nil
}

// GetValueLookupPluginHostOptions returns options relating to value handling.
func (d *lLookupPluginHost) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupPluginHostPlatformIDs returns the platform IDs to filter on.
func (d *lLookupPluginHost) LookupPluginHostPlatformIDs() []string {
	return d.platformIDs
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lLookupPluginHost) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupPluginHost)
	if !ok {
		return false
	}

	if !slices.Equal(d.LookupPluginHostPlatformIDs(), od.LookupPluginHostPlatformIDs()) {
		return false
	}

	return true
}

// GetName returns the directive's type name.
func (d *lLookupPluginHost) GetName() string {
	return "LookupPluginHost"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lLookupPluginHost) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if platformIDs := d.LookupPluginHostPlatformIDs(); len(platformIDs) != 0 {
		vals["platform-ids"] = platformIDs
	}
	return vals
}

// _ is a type assertion
var (
	_ LookupPluginHost             = ((*lLookupPluginHost)(nil))
	_ directive.DirectiveWithEquiv = ((*lLookupPluginHost)(nil))
)
