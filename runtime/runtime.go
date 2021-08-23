package runtime

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// Runtime is the environment-specific implementation of IPC and browser window management.
type Runtime interface {
	// GetLogger returns the root log entry.
	GetLogger() *logrus.Entry
	// GetBus returns the root controller bus to use in this process.
	GetBus() bus.Bus

	// GetStorage returns the set of available storage providers.
	GetStorage(ctx context.Context) ([]Storage, error)

	// GetWebViews returns the current snapshot of active WebViews.
	GetWebViews(ctx context.Context) ([]WebView, error)
	// CreateWebView creates a new web view and waits for it to become active.
	//
	// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
	CreateWebView(ctx context.Context) (WebView, error)

	// Execute executes the runtime.
	// Returns any errors, nil if Execute is not required.
	Execute(ctx context.Context) error

	// Close closes the runtime and waits for Execute to finish if wait is set.
	// if ctx is nil, don't wait for the close to complete.
	Close(ctx context.Context) error
}

// RuntimeHandler is the handler (usually runtime controller) for runtime calls.
type RuntimeHandler interface {
	// TODO
}

// RuntimeConfig is a configuration for the runtime controller.
type RuntimeConfig interface {
	// Config indicates this is a controllerbus config object.
	config.Config
}

// RuntimeController is a controller managing a runtime.
type RuntimeController interface {
	// Controller indicates this is a controller bus controller.
	controller.Controller
	// GetRuntime returns the controlled runtime, waiting for it to be non-nil.
	GetRuntime(ctx context.Context) (Runtime, error)
}

// RuntimeConstructor constructs a runtime with common parameters.
type RuntimeConstructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler RuntimeHandler,
) (Runtime, error)
