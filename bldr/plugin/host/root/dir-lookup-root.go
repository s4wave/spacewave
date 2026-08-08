package plugin_host_root

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupRoot is a directive to look up process-lifetime plugin host roots.
type LookupRoot interface {
	directive.Directive

	// LookupRootPlatformIDs filters the platform IDs to match.
	LookupRootPlatformIDs() []string
}

// LookupRootValue is the result type for LookupRoot.
type LookupRootValue = *Root

type lookupRoot struct {
	platformIDs []string
}

// NewLookupRoot constructs a new LookupRoot directive.
func NewLookupRoot(platformIDs []string) LookupRoot {
	if len(platformIDs) != 0 {
		platformIDs = slices.Clone(platformIDs)
		slices.Sort(platformIDs)
		platformIDs = slices.Compact(platformIDs)
	}
	return &lookupRoot{platformIDs: platformIDs}
}

// ExLookupRootByPlatform executes the LookupRoot directive for a single platform ID.
func ExLookupRootByPlatform(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	platformID string,
	valDisposeCallback func(),
) (LookupRootValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupRootValue](
		ctx,
		b,
		NewLookupRoot([]string{platformID}),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCallback,
		nil,
	)
}

// ExLookupRoot executes the LookupRoot directive.
func ExLookupRoot(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	platformIDs []string,
	valDisposeCallback func(),
) (LookupRootValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupRootValue](
		ctx,
		b,
		NewLookupRoot(platformIDs),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCallback,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupRoot) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupRoot) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupRootPlatformIDs returns the platform IDs to filter on.
func (d *lookupRoot) LookupRootPlatformIDs() []string {
	return d.platformIDs
}

// IsEquivalent checks if the other directive is equivalent.
func (d *lookupRoot) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupRoot)
	if !ok {
		return false
	}
	return slices.Equal(d.LookupRootPlatformIDs(), od.LookupRootPlatformIDs())
}

// GetName returns the directive's type name.
func (d *lookupRoot) GetName() string {
	return "LookupPluginHostRoot"
}

// GetDebugVals returns the directive debug values.
func (d *lookupRoot) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if platformIDs := d.LookupRootPlatformIDs(); len(platformIDs) != 0 {
		vals["platform-ids"] = platformIDs
	}
	return vals
}

// _ is a type assertion
var (
	_ LookupRoot                   = (*lookupRoot)(nil)
	_ directive.DirectiveWithEquiv = (*lookupRoot)(nil)
)
