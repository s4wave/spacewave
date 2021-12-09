package web

import (
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/bldr/runtime/ipc"
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// Remote is a remote instance of a web runtime.
//
// Communicates with the frontend using src/bldr/runtime.ts
type Remote struct {
	id  string
	le  *logrus.Entry
	bus bus.Bus
	st  []storage.Storage
	ipc ipc.IPC

	// trig is triggered when any below fields change
	trig chan struct{}
	// mtx guards below fields
	mtx sync.Mutex
	// state contains the current state, or nil if not resolved
	// only editable while mtx is locked
	state *rState
}

// rState contains information about the Remote controller
type rState struct {
	// ctx is the root context
	ctx context.Context
	// webViews is the current list of web views
	webViews []*RemoteWebView
}

// NewRemote constructs a new browser runtime.
//
// id should be the runtime identifier specified at startup by the js loader.
// initWebView should be a handle to the WebView which created the Remote.
func NewRemote(le *logrus.Entry, b bus.Bus, id string, st []storage.Storage, ipc ipc.IPC) (*Remote, error) {
	return &Remote{
		id:  id,
		le:  le,
		bus: b,
		st:  st,
		ipc: ipc,

		trig: make(chan struct{}, 1),
	}, nil
}

// GetLogger returns the root log entry.
func (r *Remote) GetLogger() *logrus.Entry {
	return r.le
}

// GetBus returns the root controller bus to use in this process.
func (r *Remote) GetBus() bus.Bus {
	return r.bus
}

// GetStorage returns the set of available storage providers.
func (r *Remote) GetStorage(ctx context.Context) ([]storage.Storage, error) {
	st := make([]storage.Storage, len(r.st))
	copy(st, r.st)
	return st, nil
}

// GetWebViews returns the current snapshot of active WebViews.
func (r *Remote) GetWebViews(ctx context.Context) ([]runtime.WebView, error) {
	var out []runtime.WebView
	err := r.waitState(ctx, func(s *rState) error {
		out = make([]runtime.WebView, len(s.webViews))
		for i := range s.webViews {
			out[i] = s.webViews[i]
		}
		return nil
	})
	return out, err
}

// CreateWebView creates a new web view and waits for it to become active.
//
// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
func (r *Remote) CreateWebView(ctx context.Context) (runtime.WebView, error) {
	var out runtime.WebView
	err := r.waitState(ctx, func(s *rState) error {
		var err error
		// out, err = s.remote.CreateWebView(ctx)
		err = errors.New("TODO CreateWebView against remote webview runtime")
		return err
	})
	return out, err
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Remote) Execute(ctx context.Context) error {
	le := r.le
	le.Infof("remote runtime starting up: %v", r.id)

	// write query view status
	if err := r.WriteMessage(NewQueryWebStatus()); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

// WriteMessage writes a proto message to the stream.
func (r *Remote) WriteMessage(msg *RuntimeToWeb) error {
	dat, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = r.ipc.Write([]byte(dat))
	return err
}

// Close closes the runtime and waits for Execute to finish if ctx is provided
func (r *Remote) Close(ctx context.Context) error {
	// close all windows
	r.mtx.Lock()
	r.state = nil
	r.mtx.Unlock()
	return nil
}

// pushState triggers all waiters.
// expects mtx to be locked
func (r *Remote) pushState(s *rState) {
	r.state = s
	for {
		select {
		case r.trig <- struct{}{}:
		default:
			return
		}
	}
}

// waitState waits for the state to be ready.
func (r *Remote) waitState(ctx context.Context, cb func(s *rState) error) error {
	for {
		var err error
		r.mtx.Lock()
		st := r.state
		if st != nil {
			err = cb(st)
		}
		r.mtx.Unlock()
		if err != nil || st != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.trig:
		}
	}
}

// _ is a type assertion
var _ runtime.Runtime = ((*Remote)(nil))
