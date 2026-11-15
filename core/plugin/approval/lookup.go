package plugin_approval

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupPluginApprovals is a directive to look up the device's plugin
// decisions. The device's one Controller resolves it.
type LookupPluginApprovals interface {
	// Directive indicates LookupPluginApprovals is a directive.
	directive.Directive

	// IsLookupPluginApprovals marks the directive type.
	IsLookupPluginApprovals()
}

// LookupPluginApprovalsValue is the result type for LookupPluginApprovals.
type LookupPluginApprovalsValue = *Approvals

// lookupPluginApprovals implements LookupPluginApprovals.
type lookupPluginApprovals struct{}

// NewLookupPluginApprovals constructs a LookupPluginApprovals directive.
func NewLookupPluginApprovals() LookupPluginApprovals {
	return &lookupPluginApprovals{}
}

// ExLookupPluginApprovals waits for the device's plugin decisions.
func ExLookupPluginApprovals(ctx context.Context, b bus.Bus) (*Approvals, directive.Reference, error) {
	val, _, ref, err := bus.ExecWaitValue[LookupPluginApprovalsValue](ctx, b, NewLookupPluginApprovals(), nil, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return val, ref, nil
}

// IsLookupPluginApprovals marks the directive type.
func (d *lookupPluginApprovals) IsLookupPluginApprovals() {}

// Validate validates the directive.
func (d *lookupPluginApprovals) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupPluginApprovals) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent reports whether other is also a LookupPluginApprovals.
func (d *lookupPluginApprovals) IsEquivalent(other directive.Directive) bool {
	_, ok := other.(LookupPluginApprovals)
	return ok
}

// GetName returns the directive's type name.
func (d *lookupPluginApprovals) GetName() string {
	return "LookupPluginApprovals"
}

// GetDebugVals returns the directive arguments stringified.
func (d *lookupPluginApprovals) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var (
	_ LookupPluginApprovals        = (*lookupPluginApprovals)(nil)
	_ directive.DirectiveWithEquiv = (*lookupPluginApprovals)(nil)
)
