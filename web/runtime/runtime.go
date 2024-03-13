package web_runtime

import (
	"context"

	web_document "github.com/aperturerobotics/bldr/web/document"
	sw "github.com/aperturerobotics/bldr/web/runtime/sw"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

// WebRuntime manages a list of WebDocument instances.
type WebRuntime interface {
	// Execute executes the runtime.
	// Returns any errors, nil if Execute is not required.
	// This should only be called by the web runtime controller!
	Execute(ctx context.Context) error

	// GetWebRuntimeStatusCtr contains a full snapshot of the web runtime status.
	GetWebRuntimeStatusCtr() *ccontainer.CContainer[*WebRuntimeStatus]

	// GetWebDocuments returns the current snapshot of active WebDocuments.
	GetWebDocuments(ctx context.Context) (map[string]web_document.WebDocument, error)

	// GetWebDocument waits for the remote to be ready & returns the given WebDocument.
	// If wait is set, waits for the web document ID to exist.
	// Otherwise, returns nil, nil if not found.
	GetWebDocument(ctx context.Context, webDocumentID string, wait bool) (web_document.WebDocument, error)

	// GetWebDocumentOpenStream returns a OpenStreamFunc for the given WebDocument ID.
	//
	// note: when opening the stream, waits for the given web document to exist.
	GetWebDocumentOpenStream(webDocumentID string) srpc.OpenStreamFunc

	// WaitReady waits for the state to be ready.
	WaitReady(ctx context.Context) error

	// WaitFirstWebDocument waits for at least one WebDocument to exist.
	WaitFirstWebDocument(ctx context.Context) (web_document.WebDocument, error)

	// CreateWebDocument creates a new WebDocument.
	// This usually corresponds to creating a new Tab or Window.
	//
	// Returns ErrWebDocumentUnavailable if WebDocument is not available or cannot be created.
	CreateWebDocument(ctx context.Context, webViewID string) (bool, error)

	// GetWebWorkerOpenStream returns a OpenStreamFunc for the given WebWorker ID.
	//
	// note: when opening the stream, waits for the given web worker to exist.
	GetWebWorkerOpenStream(webWorkerID string) srpc.OpenStreamFunc
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
