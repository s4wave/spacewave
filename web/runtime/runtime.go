package web_runtime

import (
	"context"

	web_document "github.com/aperturerobotics/bldr/web/document"
	sw "github.com/aperturerobotics/bldr/web/runtime/sw"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// WebRuntime manages a list of WebDocument instances.
type WebRuntime interface {
	// Execute executes the runtime.
	// Returns any errors, nil if Execute is not required.
	Execute(ctx context.Context) error

	// GetWebDocuments returns the current snapshot of active WebDocuments.
	GetWebDocuments(ctx context.Context) (map[string]web_document.WebDocument, error)

	// CreateWebDocument creates a new WebDocument and waits for it to become active.
	// This usually corresponds to creating a new Tab or Window.
	//
	// Returns ErrWebDocumentUnavailable if WebDocument is not available or cannot be created.
	CreateWebDocument(ctx context.Context, webViewID string) (web_document.WebDocument, error)

	// Close closes the runtime & all views.
	// if ctx is canceled, return before confirming all views are closed.
	Close(ctx context.Context) error
}

// WebRuntimeHandler is the handler (usually WebRuntimeController) for the document.
type WebRuntimeHandler interface {
	// ServiceWorkerHandler includes the handlers for the Fetch requests.
	sw.ServiceWorkerHandler
	// HandleWebDocument handles an incoming WebDocument.
	HandleWebDocument(web_document.WebDocument)
}

// RuntimeConfig is a configuration for the runtime controller.
type WebRuntimeConfig interface {
	// Config indicates this is a controllerbus config object.
	config.Config
}

// WebRuntimeController is a controller managing a WebRuntime.
type WebRuntimeController interface {
	// Controller indicates this is a controller bus controller.
	controller.Controller
	// GetWebRuntime returns the controlled runtime, waiting for it to be non-nil.
	GetWebRuntime(ctx context.Context) (WebRuntime, error)
}

// WebRuntimeConstructor constructs a runtime with common parameters.
type WebRuntimeConstructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler WebRuntimeHandler,
) (WebRuntime, error)

// NewWatchWebRuntimeStatusRequest constructs a new message to watch for WebRuntime status changes.
func NewWatchWebRuntimeStatusRequest() *WatchWebRuntimeStatusRequest {
	return &WatchWebRuntimeStatusRequest{}
}
