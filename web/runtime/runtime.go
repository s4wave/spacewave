package web_runtime

import (
	"context"

	fetch "github.com/aperturerobotics/bldr/web/fetch"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// WebRuntime is the environment-specific implementation of IPC and browser window management.
type WebRuntime interface {
	// Execute executes the runtime.
	// Returns any errors, nil if Execute is not required.
	Execute(ctx context.Context) error

	// GetWebViews returns the current snapshot of active WebViews.
	GetWebViews(ctx context.Context) (map[string]WebView, error)

	// CreateWebView creates a new web view and waits for it to become active.
	//
	// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
	CreateWebView(ctx context.Context, webViewID string) (WebView, error)

	// Close closes the runtime & all views.
	// if ctx is canceled, return before confirming all views are closed.
	Close(ctx context.Context) error
}

// WebRuntimeHandler is the handler (usually runtime controller) for the runtime.
type WebRuntimeHandler interface {
	// HandleFetch handles an incoming Fetch request from the web runtime.
	// The Client ID can be used to distinguish between windows / browser tabs.
	HandleFetch(strm fetch.SRPCFetchService_FetchStream) error
	// HandleWebView handles an incoming WebView on a new Goroutine.
	HandleWebView(view WebView)
}

// RuntimeConfig is a configuration for the runtime controller.
type WebRuntimeConfig interface {
	// Config indicates this is a controllerbus config object.
	config.Config
}

// RuntimeController is a controller managing a runtime.
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

// NewWatchWebStatusRequest constructs a new message to watch for web status changes.
func NewWatchWebStatusRequest() *WatchWebStatusRequest {
	return &WatchWebStatusRequest{}
}
