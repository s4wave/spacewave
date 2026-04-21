package web_pkg

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupWebPkg is a directive to lookup a WebPkg.
type LookupWebPkg interface {
	// Directive indicates LookupWebPkg is a directive.
	directive.Directive

	// LookupWebPkgID is the web package ID to lookup.
	// E.x.: "react" or "react-dom" or "@myorg/mypkg".
	// Cannot be empty.
	LookupWebPkgID() string
}

// LookupWebPkgValue is the result of LookupWebPkg.
type LookupWebPkgValue = WebPkg

// lookupWebPkg implements LookupWebPkg
type lookupWebPkg struct {
	webPkgID string
}

// NewLookupWebPkg constructs a new LookupWebPkg directive.
func NewLookupWebPkg(webPkgID string) LookupWebPkg {
	return &lookupWebPkg{webPkgID: webPkgID}
}

// ExLookupWebPkg looks up a web pkg by id.
func ExLookupWebPkg(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	webPkgID string,
) (LookupWebPkgValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupWebPkgValue](
		ctx,
		b,
		NewLookupWebPkg(webPkgID),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupWebPkg) Validate() error {
	if d.webPkgID == "" {
		return ErrEmptyPkgID
	}
	return nil
}

// GetValueLookupWebPkgOptions returns options relating to value handling.
func (d *lookupWebPkg) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupWebPkg is the web pkg ID to lookup.
func (d *lookupWebPkg) LookupWebPkgID() string {
	return d.webPkgID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupWebPkg) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupWebPkg)
	if !ok {
		return false
	}

	if d.LookupWebPkgID() != od.LookupWebPkgID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupWebPkg) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWebPkg) GetName() string {
	return "LookupWebPkg"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupWebPkg) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["web-pkg-id"] = []string{d.LookupWebPkgID()}
	return vals
}

// _ is a type assertion
var _ LookupWebPkg = ((*lookupWebPkg)(nil))
