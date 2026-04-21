package web_document

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupWebDocument is a directive to lookup a WebDocument.
type LookupWebDocument interface {
	// Directive indicates LookupWebDocument is a directive.
	directive.Directive

	// LookupWebDocumentID is the web view ID to lookup.
	// Cannot be empty.
	LookupWebDocumentID() string
	// LookupWebDocumentWait indicates we should wait for the view to exist.
	//
	// If unset, we expect the doc already exists, and will not be recreated
	// after it is deleted (thus pointless to wait if not found).
	LookupWebDocumentWait() bool
}

// LookupWebDocumentValue is the result of LookupWebDocument.
type LookupWebDocumentValue = WebDocument

// lookupWebDocument implements LookupWebDocument
type lookupWebDocument struct {
	webViewID string
	wait      bool
}

// NewLookupWebDocument constructs a new LookupWebDocument directive.
func NewLookupWebDocument(webViewID string, wait bool) LookupWebDocument {
	return &lookupWebDocument{webViewID: webViewID, wait: wait}
}

// ExLookupWebDocument looks up a web view by id.
//
// wait waits for the web view to exist.
func ExLookupWebDocument(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	webViewID string,
	wait bool,
) (LookupWebDocumentValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupWebDocumentValue](
		ctx,
		b,
		NewLookupWebDocument(webViewID, wait),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupWebDocument) Validate() error {
	if d.webViewID == "" {
		return ErrEmptyWebDocumentID
	}
	return nil
}

// GetValueLookupWebDocumentOptions returns options relating to value handling.
func (d *lookupWebDocument) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupWebDocument is the web view ID to lookup.
func (d *lookupWebDocument) LookupWebDocumentID() string {
	return d.webViewID
}

// LookupWebDocumentWait indicates we should wait for the view to exist.
//
// If unset, we expect the view already exists, and will not be recreated
// after it is deleted (thus pointless to wait if not found).
func (d *lookupWebDocument) LookupWebDocumentWait() bool {
	return d.wait
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupWebDocument) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupWebDocument)
	if !ok {
		return false
	}

	if d.LookupWebDocumentID() != od.LookupWebDocumentID() {
		return false
	}
	if d.LookupWebDocumentWait() != od.LookupWebDocumentWait() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupWebDocument) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWebDocument) GetName() string {
	return "LookupWebDocument"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupWebDocument) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["document-id"] = []string{d.LookupWebDocumentID()}
	return vals
}

// _ is a type assertion
var _ LookupWebDocument = ((*lookupWebDocument)(nil))
