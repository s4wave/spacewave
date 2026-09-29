package bldr_web_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
)

// Desktop opens or focuses the desktop shell owned by this plugin.
type Desktop interface {
	// OpenOrFocusMainWindow acknowledges Electron main and returns its shell generation.
	OpenOrFocusMainWindow(ctx context.Context, req *OpenOrFocusDesktopRequest) (uint64, error)
	// DesktopPresence returns one generation's state, or nil for an older ended generation.
	// The returned container retains its terminal result across later opens.
	DesktopPresence(generation uint64) *ccontainer.CContainer[*WatchDesktopPresenceResponse]
}

// LookupDesktop resolves the plugin's desktop controller without starting it.
type LookupDesktop interface {
	// Directive indicates LookupDesktop is a directive.
	directive.Directive

	// IsLookupDesktop marks the directive type.
	IsLookupDesktop()
}

// lookupDesktop is the single desktop lookup directive.
type lookupDesktop struct{}

// NewLookupDesktop constructs a lookup of the plugin's desktop controller.
func NewLookupDesktop() LookupDesktop {
	return &lookupDesktop{}
}

// ExLookupDesktop resolves the plugin's desktop controller. It returns nil
// after an idle lookup when this plugin has no Electron capability. The caller
// releases a non-nil reference after using the controller.
func ExLookupDesktop(ctx context.Context, b bus.Bus) (Desktop, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[Desktop](ctx, b, NewLookupDesktop(), bus.ReturnIfIdle(true), nil, nil)
}

// IsLookupDesktop marks the directive type.
func (d *lookupDesktop) IsLookupDesktop() {}

// Validate accepts the desktop lookup.
func (d *lookupDesktop) Validate() error {
	return nil
}

// GetValueOptions uses the default directive value lifetime.
func (d *lookupDesktop) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent merges concurrent desktop lookups.
func (d *lookupDesktop) IsEquivalent(other directive.Directive) bool {
	_, ok := other.(LookupDesktop)
	return ok
}

// Superceeds retains an existing desktop lookup.
func (d *lookupDesktop) Superceeds(other directive.Directive) bool {
	return false
}

// GetName identifies this directive in the controller bus.
func (d *lookupDesktop) GetName() string {
	return "LookupDesktop"
}

// GetDebugVals reports no arguments for the desktop lookup.
func (d *lookupDesktop) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{}
}

// _ is a type assertion.
var _ LookupDesktop = (*lookupDesktop)(nil)
