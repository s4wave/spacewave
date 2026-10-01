package web_document_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
)

// resolveLookupWebDocument resolves a LookupWebDocument directive.
func (c *Controller) resolveLookupWebDocument(
	_ context.Context,
	_ directive.Instance,
	dir web_document.LookupWebDocument,
) ([]directive.Resolver, error) {
	return directive.R(&lookupWebDocumentResolver{c: c, d: dir}, nil)
}

// lookupWebDocumentResolver resolves LookupWebDocument with the controller.
type lookupWebDocumentResolver struct {
	// c is the controller
	c *Controller
	// d is the directive
	d web_document.LookupWebDocument
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupWebDocumentResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Load the registered web document from the controller.
	doc := r.c.GetWebDocument()

	// Skip when the directive targets a different web document ID.
	lookupWebDocID := r.d.LookupWebDocumentID()
	if lookupWebDocID != "" && lookupWebDocID != doc.GetWebDocumentUuid() {
		return nil
	}

	// Emit the resolved web document value to the handler.
	val := doc
	_, _ = handler.AddValue(val)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = (*lookupWebDocumentResolver)(nil)
