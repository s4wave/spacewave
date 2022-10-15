package web_document_controller

import (
	"context"
	"strings"

	web_document "github.com/aperturerobotics/bldr/web/document"
	web_view "github.com/aperturerobotics/bldr/web/document/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a runtime with common parameters.
type Constructor func(
	le *logrus.Entry,
	bus bus.Bus,
	handler web_document.WebDocumentHandler,
	webDocumentId string,
) (web_document.WebDocument, error)

// Controller implements a common bldr WebDocument controller.
// Tracks attached WebDocument state and manages RPC calls in/out.
type Controller struct {
	// ctx is the controller context
	// set in the execute() function
	// ensure not used before execute sets it.
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus

	// doc is the web document instance
	doc web_document.WebDocument
	// webDocumentId is the controller id to use
	webDocumentId string
	// webDocumentVersion is the version
	webDocumentVersion semver.Version
}

// NewController constructs a new runtime controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	webDocumentId string,
	webDocumentVersion semver.Version,
	ctor Constructor,
) (*Controller, error) {
	ctrl := &Controller{
		le:  le.WithField("document-id", webDocumentId),
		bus: bus,

		webDocumentId:      webDocumentId,
		webDocumentVersion: webDocumentVersion,
	}
	var err error
	ctrl.doc, err = ctor(le, bus, ctrl, webDocumentId)
	if err != nil {
		return nil, err
	}
	return ctrl, nil
}

// GetWebDocument returns the controlled WebDocument.
func (c *Controller) GetWebDocument() web_document.WebDocument {
	return c.doc
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return strings.Join([]string{
		"bldr",
		"document",
		c.webDocumentId,
		c.webDocumentVersion.String(),
	}, "/")
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		c.GetControllerID(),
		c.webDocumentVersion,
		"WebDocument "+c.webDocumentId+"@"+c.webDocumentVersion.String(),
	)
}

// Execute executes the WebDocument controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	c.ctx = ctx
	defer ctxCancel()

	c.le.Debug("executing web document controller")
	return c.doc.Execute(ctx)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// HandleWebView handles an incoming WebView on a new Goroutine.
func (c *Controller) HandleWebView(wv web_view.WebView) {
	loadTestComponent(c.ctx, c.le, wv)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	// return c.doc.Close(c.ctx)
	return nil
}

// _ is a type assertion
var (
	_ web_document.WebDocumentController = ((*Controller)(nil))
	_ web_document.WebDocumentHandler    = ((*Controller)(nil))
)
