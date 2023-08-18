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
	// Note that the package ID can have slashes.
	// When used in URLs we use url encoding: /b/pkg/%40myorg%2Fmypkg/...
	// Cannot be empty.
	LookupWebPkgID() string

	// LookupWebPkgWait indicates we should wait for the WebPkg to be found.
	//
	// Set this if you want the web package lookup request to wait (hang) until
	// a plugin is added resolving the lookup or the request is canceled.
	LookupWebPkgWait() bool
}

// LookupWebPkgValue is the result of LookupWebPkg.
type LookupWebPkgValue = WebPkg

// lookupWebPkg implements LookupWebPkg
type lookupWebPkg struct {
	webPkgID string
	wait     bool
}

// NewLookupWebPkg constructs a new LookupWebPkg directive.
func NewLookupWebPkg(webPkgID string, wait bool) LookupWebPkg {
	return &lookupWebPkg{webPkgID: webPkgID, wait: wait}
}

// ExLookupWebPkg looks up a web view by id.
//
// wait waits for the web view to exist.
func ExLookupWebPkg(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	webPkgID string,
	wait bool,
) (LookupWebPkgValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupWebPkgValue](
		ctx,
		b,
		NewLookupWebPkg(webPkgID, wait),
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

// LookupWebPkg is the web view ID to lookup.
func (d *lookupWebPkg) LookupWebPkgID() string {
	return d.webPkgID
}

// LookupWebPkgWait indicates we should wait for the web pkg to exist.
//
// Otherwise returns 404 if not found.
func (d *lookupWebPkg) LookupWebPkgWait() bool {
	return d.wait
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

	if d.LookupWebPkgWait() != od.LookupWebPkgWait() {
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
