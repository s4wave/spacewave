package web_document_controller

import (
	"context"

	web_document "github.com/aperturerobotics/bldr/web/document"
	"github.com/aperturerobotics/controllerbus/directive"
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
	doc := r.c.GetWebDocument()

	lookupWebDocID := r.d.LookupWebDocumentID()
	if lookupWebDocID != "" && lookupWebDocID != doc.GetWebDocumentUuid() {
		return nil
	}

	var val web_document.LookupWebDocumentValue = doc
	_, _ = handler.AddValue(val)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupWebDocumentResolver)(nil))
