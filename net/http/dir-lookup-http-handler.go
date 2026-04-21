package bifrost_http

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// LookupHTTPHandler is a directive to lookup a HTTP handler.
type LookupHTTPHandler interface {
	// Directive indicates LookupHTTPHandler is a directive.
	directive.Directive

	// LookupHTTPHandlerMethod is the method string for the request.
	// Can be empty to allow any.
	LookupHTTPHandlerMethod() string

	// LookupHTTPHandlerURL is the URL for the request.
	LookupHTTPHandlerURL() *url.URL

	// LookupHTTPHandlerClientID is a string identifying the client.
	// Can be empty.
	LookupHTTPHandlerClientID() string
}

// LookupHTTPHandlerValue is the result type for LookupHTTPHandler.
// Multiple results may be pushed to the directive.
type LookupHTTPHandlerValue = http.Handler

// lookupHTTPHandler implements LookupHTTPHandler
type lookupHTTPHandler struct {
	handlerMethod string
	handlerURL    *url.URL
	clientID      string
}

// NewLookupHTTPHandler constructs a new LookupHTTPHandler directive.
// handlerMethod can be empty to allow any.
func NewLookupHTTPHandler(handlerMethod string, handlerURL *url.URL, clientID string) LookupHTTPHandler {
	return &lookupHTTPHandler{handlerMethod: handlerMethod, handlerURL: handlerURL, clientID: clientID}
}

// ExLookupHTTPHandlers executes the LookupHTTPHandler directive.
// If waitOne is set, waits for at least one value before returning.
// handlerMethod can be empty to allow any.
func ExLookupHTTPHandlers(
	ctx context.Context,
	b bus.Bus,
	handlerMethod string,
	handlerURL *url.URL,
	clientID string,
	waitOne bool,
) ([]LookupHTTPHandlerValue, directive.Instance, directive.Reference, error) {
	return bus.ExecCollectValues[LookupHTTPHandlerValue](
		ctx,
		b,
		NewLookupHTTPHandler(handlerMethod, handlerURL, clientID),
		waitOne,
		nil,
	)
}

// ExLookupFirstHTTPHandler waits for the first HTTP handler to be returned.
// if returnIfIdle is set and the directive becomes idle, returns nil, nil, nil,
// handlerMethod can be empty to allow any.
func ExLookupFirstHTTPHandler(
	ctx context.Context,
	b bus.Bus,
	handlerMethod string,
	handlerURL *url.URL,
	clientID string,
	returnIfIdle bool,
	valDisposeCb func(),
) (LookupHTTPHandlerValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupHTTPHandlerValue](
		ctx,
		b,
		NewLookupHTTPHandler(handlerMethod, handlerURL, clientID),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
		nil,
	)
}

// MatchServeMuxPattern matches a LookupHTTPMethod against at ServeMux.
func MatchServeMuxPattern(mux *http.ServeMux, dir LookupHTTPHandler) (handler http.Handler, pattern string) {
	method := dir.LookupHTTPHandlerMethod()
	if method == "" {
		method = "OPTIONS"
	}
	return mux.Handler(&http.Request{Method: method, URL: dir.LookupHTTPHandlerURL()})
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupHTTPHandler) Validate() error {
	if d.handlerURL == nil {
		return errors.New("handler url cannot be nil")
	}
	return nil
}

// GetValueLookupHTTPHandlerOptions returns options relating to value handling.
func (d *lookupHTTPHandler) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 250,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupHTTPHandlerMethod is the method string for the request.
// Can be empty.
func (d *lookupHTTPHandler) LookupHTTPHandlerMethod() string {
	return d.handlerMethod
}

// LookupHTTPHandlerURL is the URL for the request.
// Cannot be empty.
func (d *lookupHTTPHandler) LookupHTTPHandlerURL() *url.URL {
	return d.handlerURL
}

// LookupHTTPHandlerClientID returns the client id.
// Can be empty.
func (d *lookupHTTPHandler) LookupHTTPHandlerClientID() string {
	return d.clientID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupHTTPHandler) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupHTTPHandler)
	if !ok {
		return false
	}

	if d.LookupHTTPHandlerMethod() != od.LookupHTTPHandlerMethod() {
		return false
	}
	if d.LookupHTTPHandlerURL().String() != od.LookupHTTPHandlerURL().String() {
		return false
	}
	if d.LookupHTTPHandlerClientID() != od.LookupHTTPHandlerClientID() {
		return false
	}
	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupHTTPHandler) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupHTTPHandler) GetName() string {
	return "LookupHTTPHandler"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupHTTPHandler) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if method := d.LookupHTTPHandlerMethod(); method != "" {
		vals["method"] = []string{method}
	}
	vals["url"] = []string{d.LookupHTTPHandlerURL().String()}
	if clientID := d.LookupHTTPHandlerClientID(); clientID != "" {
		vals["client-id"] = []string{clientID}
	}
	return vals
}

// _ is a type assertion
var _ LookupHTTPHandler = ((*lookupHTTPHandler)(nil))
