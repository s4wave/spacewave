package cdn_bstore

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// LookupCdnSpaceGrants resolves the GrantSource for one private CDN Space.
// Value: GrantSource. The app that holds the Space's access resolves it.
type LookupCdnSpaceGrants interface {
	directive.Directive
	// LookupCdnSpaceGrantsSpaceID is the Space whose grants are requested.
	LookupCdnSpaceGrantsSpaceID() string
}

// lookupCdnSpaceGrants names one Space's grant source.
type lookupCdnSpaceGrants struct {
	spaceID string
}

// NewLookupCdnSpaceGrants constructs a lookup for spaceID's grant source.
func NewLookupCdnSpaceGrants(spaceID string) LookupCdnSpaceGrants {
	return &lookupCdnSpaceGrants{spaceID: spaceID}
}

// ExLookupCdnSpaceGrants returns spaceID's grant source and a release for
// the reference that holds it. It waits until a resolver supplies one.
func ExLookupCdnSpaceGrants(ctx context.Context, b bus.Bus, spaceID string, released func()) (GrantSource, func(), error) {
	value, _, ref, err := bus.ExecOneOffTyped[GrantSource](ctx, b, NewLookupCdnSpaceGrants(spaceID), nil, released)
	if err != nil {
		return nil, nil, err
	}
	return value.GetValue(), ref.Release, nil
}

// LookupCdnSpaceGrantsSpaceID returns the Space.
func (d *lookupCdnSpaceGrants) LookupCdnSpaceGrantsSpaceID() string { return d.spaceID }

// Validate requires a Space.
func (d *lookupCdnSpaceGrants) Validate() error {
	if d.spaceID == "" {
		return errors.New("a grant lookup needs a Space ID")
	}
	return nil
}

// GetValueOptions disposes the lookup when its caller releases it.
func (d *lookupCdnSpaceGrants) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent merges lookups for the same Space.
func (d *lookupCdnSpaceGrants) IsEquivalent(other directive.Directive) bool {
	o, ok := other.(LookupCdnSpaceGrants)
	return ok && o.LookupCdnSpaceGrantsSpaceID() == d.spaceID
}

// Superceeds preserves other lookups.
func (d *lookupCdnSpaceGrants) Superceeds(directive.Directive) bool { return false }

// GetName names the directive.
func (d *lookupCdnSpaceGrants) GetName() string { return "LookupCdnSpaceGrants" }

// GetDebugVals names the Space and no credential.
func (d *lookupCdnSpaceGrants) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"space-id": []string{d.spaceID}}
}

// _ is a type assertion
var _ LookupCdnSpaceGrants = (*lookupCdnSpaceGrants)(nil)
