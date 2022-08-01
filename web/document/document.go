package web_document

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/document/view"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// WebDocument is a tree of WebView managed separately from other WebDocument instances.
type WebDocument interface {
	// GetWebDocumentUuid returns the web document identifier.
	GetWebDocumentUuid() string

	// Execute executes the runtime.
	// Returns any errors, nil if Execute is not required.
	Execute(ctx context.Context) error

	// GetWebViews returns the current snapshot of active WebViews.
	GetWebViews(ctx context.Context) (map[string]web_view.WebView, error)

	// CreateWebView creates a new web view and waits for it to become active.
	//
	// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
	CreateWebView(ctx context.Context, webViewID string) (web_view.WebView, error)

	// Close closes the runtime & all views.
	// if ctx is canceled, return before confirming all views are closed.
	Close(ctx context.Context) error
}

// WebDocumentHandler is the handler (usually WebDocumentController) for the document.
type WebDocumentHandler interface {
	// HandleWebView handles an incoming WebView on a new Goroutine.
	HandleWebView(view web_view.WebView)
}

// RuntimeConfig is a configuration for the runtime controller.
type WebDocumentConfig interface {
	// Config indicates this is a controllerbus config object.
	config.Config
}

// WebDocumentController is a controller managing a WebDocument.
type WebDocumentController interface {
	// Controller indicates this is a controller bus controller.
	controller.Controller
	// GetWebDocument returns the controlled WebDocument.
	GetWebDocument() WebDocument
}

// WebDocumentConstructor constructs a runtime with common parameters.
type WebDocumentConstructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler WebDocumentHandler,
) (WebDocument, error)

// NewWatchWebDocumentStatusRequest constructs a new message to watch for WebDocument status changes.
func NewWatchWebDocumentStatusRequest() *WatchWebDocumentStatusRequest {
	return &WatchWebDocumentStatusRequest{}
}
