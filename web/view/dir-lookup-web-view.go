package web_view

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupWebView is a directive to lookup a WebView.
type LookupWebView interface {
	// Directive indicates LookupWebView is a directive.
	directive.Directive

	// LookupWebViewID is the web view ID to lookup.
	// Cannot be empty.
	LookupWebViewID() string
	// LookupWebViewWait indicates we should wait for the view to exist.
	//
	// If unset, we expect the view already exists, and will not be recreated
	// after it is deleted (thus pointless to wait if not found).
	LookupWebViewWait() bool
}

// LookupWebViewValue is the result of LookupWebView.
type LookupWebViewValue = WebView

// lookupWebView implements LookupWebView
type lookupWebView struct {
	webViewID string
	wait      bool
}

// NewLookupWebView constructs a new LookupWebView directive.
func NewLookupWebView(webViewID string, wait bool) LookupWebView {
	return &lookupWebView{webViewID: webViewID, wait: wait}
}

// ExLookupWebView looks up a web view by id.
//
// wait waits for the web view to exist.
func ExLookupWebView(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	webViewID string,
	wait bool,
	valDisposeCallback func(),
) (LookupWebViewValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupWebViewValue](
		ctx,
		b,
		NewLookupWebView(webViewID, wait),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCallback,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupWebView) Validate() error {
	if d.webViewID == "" {
		return ErrEmptyWebViewID
	}
	return nil
}

// GetValueLookupWebViewOptions returns options relating to value handling.
func (d *lookupWebView) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupWebView is the web view ID to lookup.
func (d *lookupWebView) LookupWebViewID() string {
	return d.webViewID
}

// LookupWebViewWait indicates we should wait for the view to exist.
//
// If unset, we expect the view already exists, and will not be recreated
// after it is deleted (thus pointless to wait if not found).
func (d *lookupWebView) LookupWebViewWait() bool {
	return d.wait
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupWebView) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupWebView)
	if !ok {
		return false
	}

	if d.LookupWebViewID() != od.LookupWebViewID() {
		return false
	}
	if d.LookupWebViewWait() != od.LookupWebViewWait() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupWebView) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWebView) GetName() string {
	return "LookupWebView"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupWebView) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["view-id"] = []string{d.LookupWebViewID()}
	return vals
}

// _ is a type assertion
var _ LookupWebView = ((*lookupWebView)(nil))
