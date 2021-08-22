package electron

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/bldr/runtime/core"
	storage "github.com/aperturerobotics/bldr/target/electron/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// Runtime is the electron runtime.
//
// Communicates with the electron Renderer via IPC.
type Runtime struct {
	ctx context.Context
	le  *logrus.Entry
	bus bus.Bus

	electronPath, rendererPath string
	storage                    []runtime.Storage
	execSema                   *semaphore.Weighted

	// mtx guards below fields
	mtx sync.Mutex
	// webViews contains the current set of web views
	webViews []runtime.WebView
}

// NewRuntime constructs a new browser runtime.
// TODO: pass electron instance instead of path to electron
func NewRuntime(ctx context.Context, le *logrus.Entry, electronPath, rendererPath string) (*Runtime, error) {
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}
	st := storage.BuildStorage(b, sr)
	return &Runtime{
		ctx: ctx,
		le:  le,
		bus: b,

		electronPath: electronPath,
		rendererPath: rendererPath,

		storage:  st,
		execSema: semaphore.NewWeighted(1),
	}, nil
}

// GetContext returns the root context of the environment.
func (r *Runtime) GetContext() context.Context {
	return r.ctx
}

// GetLogger returns the root log entry.
func (r *Runtime) GetLogger() *logrus.Entry {
	return r.le
}

// GetBus returns the root controller bus to use in this process.
func (r *Runtime) GetBus() bus.Bus {
	return r.bus
}

// GetStorage returns the set of available storage providers.
func (r *Runtime) GetStorage() []runtime.Storage {
	st := make([]runtime.Storage, len(r.storage))
	copy(st, r.storage)
	return st
}

// GetWebViews returns the current snapshot of active WebViews.
func (r *Runtime) GetWebViews() []runtime.WebView {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	v := make([]runtime.WebView, len(r.webViews))
	copy(v, r.webViews)
	return v
}

// CreateWebView creates a new web view and waits for it to become active.
//
// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
func (r *Runtime) CreateWebView(ctx context.Context) (runtime.WebView, error) {
	// TODO: send message to webpage to create view & wait for reply
	return nil, runtime.ErrWebViewUnavailable
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Runtime) Execute(ctx context.Context) error {
	err := r.execSema.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	defer r.execSema.Release(1)

	e, err := RunElectron(ctx, r.le, r.electronPath, r.rendererPath)
	if err != nil {
		return err
	}

	<-ctx.Done()

	r.le.Info("exiting")
	e.Close()

	return nil
}

// Close closes the runtime and waits for Execute to finish if wait is set.
// if ctx is nil, don't wait for the close to complete.
func (r *Runtime) Close(ctx context.Context) error {
	// ctx will already have been canceled;
	if ctx == nil {
		return nil
	}
	// wait for electron to exit
	err := r.execSema.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	r.execSema.Release(1)
	return nil
}

// _ is a type assertion
var _ runtime.Runtime = ((*Runtime)(nil))
