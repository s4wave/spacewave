package bldr_http

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

	// LookupHTTPHandlerURL is the URL string for the request.
	// Cannot be empty.
	LookupHTTPHandlerURL() string

	// LookupHTTPHandlerClientID is a string identifying the client.
	// Can be empty.
	LookupHTTPHandlerClientID() string
}

// LookupHTTPHandlerValue is the result type for LookupHTTPHandler.
// Multiple results may be pushed to the directive.
type LookupHTTPHandlerValue = http.Handler

// lookupHTTPHandler implements LookupHTTPHandler
type lookupHTTPHandler struct {
	handlerURL string
	clientID   string
}

// NewLookupHTTPHandler constructs a new LookupHTTPHandler directive.
func NewLookupHTTPHandler(handlerURL, clientID string) LookupHTTPHandler {
	return &lookupHTTPHandler{handlerURL: handlerURL, clientID: clientID}
}

// ExLookupHTTPHandlers executes the LookupHTTPHandler directive.
func ExLookupHTTPHandlers(
	ctx context.Context,
	b bus.Bus,
	handlerURL,
	clientID string,
) ([]LookupHTTPHandlerValue, directive.Reference, error) {
	vals, valRef, err := bus.ExecCollectValues(ctx, b, NewLookupHTTPHandler(handlerURL, clientID), nil)
	if err != nil {
		return nil, nil, err
	}
	handlers := make([]LookupHTTPHandlerValue, 0, len(vals))
	for _, val := range vals {
		hv, hvOk := val.(LookupHTTPHandlerValue)
		if hvOk && hv != nil {
			handlers = append(handlers, hv)
		}
	}
	return handlers, valRef, nil
}

// ExLookupFirstHTTPHandler waits for the first HTTP handler to be returned.
// if returnIfIdle is set and the directive becomes idle, returns nil, nil, nil,
func ExLookupFirstHTTPHandler(
	ctx context.Context,
	b bus.Bus,
	handlerURL,
	clientID string,
	returnIfIdle bool,
) (LookupHTTPHandlerValue, directive.Reference, error) {
	return bus.ExecWaitValue[LookupHTTPHandlerValue](
		ctx,
		b,
		NewLookupHTTPHandler(handlerURL, clientID),
		returnIfIdle,
		nil,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupHTTPHandler) Validate() error {
	if d.handlerURL == "" {
		return errors.New("handler url cannot be empty")
	}
	if _, err := url.Parse(d.handlerURL); err != nil {
		return errors.Wrap(err, "invalid handler url")
	}
	return nil
}

// GetValueLookupHTTPHandlerOptions returns options relating to value handling.
func (d *lookupHTTPHandler) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur: time.Second,
	}
}

// LookupHTTPHandlerURL is the URL string for the request.
// Cannot be empty.
func (d *lookupHTTPHandler) LookupHTTPHandlerURL() string {
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

	if d.LookupHTTPHandlerURL() != od.LookupHTTPHandlerURL() {
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
	vals["url"] = []string{d.LookupHTTPHandlerURL()}
	if clientID := d.LookupHTTPHandlerClientID(); clientID != "" {
		vals["client-id"] = []string{clientID}
	}
	return vals
}

// _ is a type assertion
var _ LookupHTTPHandler = ((*lookupHTTPHandler)(nil))
