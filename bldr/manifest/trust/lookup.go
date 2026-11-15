package bldr_manifest_trust

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupTrustedRoots is a directive to look up the host's trusted root set.
// The host's Controller resolves it with its one Set.
type LookupTrustedRoots interface {
	// Directive indicates LookupTrustedRoots is a directive.
	directive.Directive

	// IsLookupTrustedRoots marks the directive type.
	IsLookupTrustedRoots()
}

// LookupTrustedRootsValue is the result type for LookupTrustedRoots.
type LookupTrustedRootsValue = *Set

// lookupTrustedRoots implements LookupTrustedRoots.
type lookupTrustedRoots struct{}

// NewLookupTrustedRoots constructs a LookupTrustedRoots directive.
func NewLookupTrustedRoots() LookupTrustedRoots {
	return &lookupTrustedRoots{}
}

// ExLookupTrustedRoots waits for the host's trusted root set. With
// returnIfIdle set, it returns nil when no controller answers, as on a host
// that keeps no set.
func ExLookupTrustedRoots(ctx context.Context, b bus.Bus, returnIfIdle bool) (*Set, directive.Reference, error) {
	av, _, ref, err := bus.ExecOneOffTyped[LookupTrustedRootsValue](
		ctx,
		b,
		NewLookupTrustedRoots(),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
	)
	if err != nil || av == nil {
		return nil, ref, err
	}
	return av.GetValue(), ref, nil
}

// IsLookupTrustedRoots marks the directive type.
func (d *lookupTrustedRoots) IsLookupTrustedRoots() {}

// Validate validates the directive.
func (d *lookupTrustedRoots) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupTrustedRoots) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{UnrefDisposeDur: 0}
}

// IsEquivalent reports whether other is also a LookupTrustedRoots.
func (d *lookupTrustedRoots) IsEquivalent(other directive.Directive) bool {
	_, ok := other.(LookupTrustedRoots)
	return ok
}

// GetName returns the directive's type name.
func (d *lookupTrustedRoots) GetName() string {
	return "LookupTrustedRoots"
}

// GetDebugVals returns the directive arguments stringified.
func (d *lookupTrustedRoots) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var (
	_ LookupTrustedRoots           = (*lookupTrustedRoots)(nil)
	_ directive.DirectiveWithEquiv = (*lookupTrustedRoots)(nil)
)
