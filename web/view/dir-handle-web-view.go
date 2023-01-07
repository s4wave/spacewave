package web_view

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// HandleWebView is a directive to handle a WebView.
type HandleWebView interface {
	// Directive indicates HandleWebView is a directive.
	directive.Directive

	// HandleWebView is the web view to handle.
	// Cannot be empty.
	HandleWebView() WebView
}

// handleWebView implements HandleWebView
type handleWebView struct {
	webView WebView
}

// NewHandleWebView constructs a new HandleWebView directive.
func NewHandleWebView(webView WebView) HandleWebView {
	return &handleWebView{webView: webView}
}

// ExHandleWebView executes handling a web view with a bus.
//
// if returnIfErr is set, if any resolvers return an error, returns that error.
func ExHandleWebView(
	rctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	webView WebView,
	returnIfErr bool,
) (err error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	di, diRef, err := b.AddDirective(
		NewHandleWebView(webView),
		bus.NewCallbackHandler(nil, nil, ctxCancel),
	)
	if err != nil {
		return err
	}
	defer diRef.Release()

	errCh := make(chan error, 1)
	if returnIfErr {
		defer di.AddIdleCallback(func(errs []error) {
			for _, err := range errs {
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			}
		})()
	}

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *handleWebView) Validate() error {
	if d.webView == nil || d.webView.GetId() == "" {
		return ErrEmptyWebViewID
	}
	return nil
}

// GetValueHandleWebViewOptions returns options relating to value handling.
func (d *handleWebView) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// HandleWebView is the web view to handle.
func (d *handleWebView) HandleWebView() WebView {
	return d.webView
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *handleWebView) GetName() string {
	return "HandleWebView"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *handleWebView) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if view := d.HandleWebView(); view != nil {
		vals["view-id"] = []string{view.GetId()}
		if parentID := view.GetParentId(); parentID != "" {
			vals["view-parent-id"] = []string{parentID}
		}
		if documentID := view.GetDocumentId(); documentID != "" {
			vals["view-document-id"] = []string{documentID}
		}
		vals["view-permanent"] = []string{strconv.FormatBool(view.GetPermanent())}
	}
	return vals
}

// _ is a type assertion
var _ HandleWebView = ((*handleWebView)(nil))
