package web_view

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
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

// ExLookupWebView executes handling a web view with a bus.
//
// if wait is set: waits for a result (ignores idle).
func ExLookupWebView(
	ctx context.Context,
	b bus.Bus,
	webViewID string,
	wait bool,
) (LookupWebViewValue, directive.Reference, error) {
	av, avRef, err := bus.ExecOneOff(
		ctx,
		b,
		NewLookupWebView(webViewID, wait),
		!wait,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	val, ok := av.GetValue().(LookupWebViewValue)
	if !ok {
		avRef.Release()
		return nil, nil, errors.New("lookup web view returned unexpected value type")
	}
	return val, avRef, nil
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
		UnrefDisposeDur: time.Millisecond * 500,
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
