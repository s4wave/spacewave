// +build js

package main

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/bldr/runtime/core"
	storage "github.com/aperturerobotics/bldr/target/browser/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// Prefix is the prefix used for messaging.
var Prefix = "@aperturerobotics/bldr"

// Runtime is the browser runtime.
//
// Usually runs in a WebWorker
// Creates new WebView by communicating with the page which created the Worker.
type Runtime struct {
	ctx     context.Context
	le      *logrus.Entry
	bus     bus.Bus
	storage []runtime.Storage

	// mtx guards below fields
	mtx sync.Mutex
	// webViews contains the current set of web views
	webViews []*WebView
}

// NewRuntime constructs a new browser runtime.
//
// initWebView should be a handle to the WebView which created the Runtime.
func NewRuntime(ctx context.Context, le *logrus.Entry, initWebView *WebView) (*Runtime, error) {
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}
	st := storage.BuildStorage(b, sr)
	webViews := []*WebView{initWebView}
	return &Runtime{ctx: ctx, le: le, bus: b, storage: st, webViews: webViews}, nil
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
	v := make([]runtime.WebView, len(r.webViews))
	for i, x := range r.webViews {
		v[i] = x
	}
	r.mtx.Unlock()
	return v
}

// CreateWebView creates a new web view and waits for it to become active.
//
// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
func (r *Runtime) CreateWebView(ctx context.Context) (runtime.WebView, error) {
	return nil, runtime.ErrWebViewUnavailable
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Runtime) Execute(ctx context.Context) error {
	return nil
}

// Close closes the runtime and waits for Execute to finish if ctx is provided
func (r *Runtime) Close(ctx context.Context) error {
	// close all windows
	r.mtx.Lock()
	wv := r.webViews
	r.webViews = nil
	r.mtx.Unlock()
	for _, w := range wv {
		if w != nil {
			w.closeWindow()
		}
	}
	return nil
}

// _ is a type assertion
var _ runtime.Runtime = ((*Runtime)(nil))
