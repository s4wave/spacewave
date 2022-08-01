package web_runtime_controller

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	web_document "github.com/aperturerobotics/bldr/web/document"
	fetch "github.com/aperturerobotics/bldr/web/fetch"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/util/billyhttp"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/blang/semver"
	"github.com/go-git/go-billy/v5"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a runtime with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler web_runtime.WebRuntimeHandler,
) (web_runtime.WebRuntime, error)

// Controller implements a common bldr runtime controller.
// Tracks attached WebRuntime state and manages RPC calls in/out.
type Controller struct {
	// ctx is the controller context
	// set in the execute() function
	// ensure not used before execute sets it.
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor

	// runtimeID is the controller id to use
	runtimeID string
	// runtimeVersion is the version
	runtimeVersion semver.Version

	// trigger is pushed to when anything changes
	trigger chan struct{}
	// mtx guards the below fields
	mtx sync.Mutex
	// rt is the runtime
	rt web_runtime.WebRuntime

	// swMux is the mux serving service worker requests.
	swMux *http.ServeMux

	// demoFs is a demonstration unixfs.
	// TODO: remove
	demoFs  *unixfs.FS
	demoTb  *world_testbed.Testbed
	demoBfh *unixfs.FSHandle
	demoBfs billy.Filesystem
	demoHfs http.FileSystem
}

// NewController constructs a new runtime controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	ctor Constructor,
	runtimeID string,
	runtimeVersion semver.Version,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		ctor: ctor,

		runtimeID:      runtimeID,
		runtimeVersion: runtimeVersion,

		trigger: make(chan struct{}, 1),
	}
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return strings.Join([]string{
		"bldr",
		"runtime",
		c.runtimeID,
		c.runtimeVersion.String(),
	}, "/")
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		c.GetControllerID(),
		c.runtimeVersion,
		"bldr runtime controller "+c.runtimeID+"@"+c.runtimeVersion.String(),
	)
}

// Execute executes the runtime controller and the runtime itself.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	c.ctx = ctx
	defer ctxCancel()

	// TODO: remove the demo fs
	c.mtx.Lock()
	var err error
	c.demoFs, c.demoTb, err = buildExampleFS(ctx, c.le)
	if err != nil {
		c.mtx.Unlock()
		return err
	}
	c.demoBfh, err = c.demoFs.AddRootReference(ctx)
	if err != nil {
		c.mtx.Unlock()
		return err
	}
	c.demoBfs = unixfs.NewBillyFilesystem(c.ctx, c.demoBfh, "", time.Now())
	c.demoHfs = billyhttp.NewFileSystem(c.demoBfs, "/b/")
	c.mtx.Unlock()

	// TODO: Wait a moment for the demoHfs to settle.
	<-time.After(time.Millisecond * 10)

	// Construct the web runtime.
	rt, err := c.ctor(
		ctx,
		c.le,
		c,
	)
	if err != nil {
		return err
	}
	defer func() {
		c.mtx.Lock()
		if c.rt == rt {
			c.rt = nil
			c.doTrigger()
		}
		c.mtx.Unlock()
		_ = rt.Close(ctx)
	}()

	c.mtx.Lock()
	c.rt = rt
	c.mtx.Unlock()
	c.doTrigger()

	c.le.Debug("executing bldr web runtime")
	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.Execute(ctx)
	}()

	for {
		// note: will add case to re-sync when needed
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

// GetWebRuntime returns the controlled runtime, waiting for it to be non-nil.
func (c *Controller) GetWebRuntime(ctx context.Context) (web_runtime.WebRuntime, error) {
	for {
		c.mtx.Lock()
		rt, trig := c.rt, c.trigger
		c.mtx.Unlock()
		if rt != nil {
			return rt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-trig:
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	return nil, nil
}

// HandleFetch handles an incoming Fetch request from the web runtime.
// The Client ID can be used to distinguish between windows / browser tabs.
func (c *Controller) HandleFetch(strm fetch.SRPCFetchService_FetchStream) error {
	handler := http.FileServer(c.demoHfs)
	return fetch.HandleFetch(strm, handler.ServeHTTP)
}

// HandleWebDocument handles an incoming WebDocument on a new Goroutine.
func (c *Controller) HandleWebDocument(wv web_document.WebDocument) {
	// no-op
}

// doTrigger triggers all waiting goroutines
func (c *Controller) doTrigger() {
	for {
		select {
		case c.trigger <- struct{}{}:
		default:
			return
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.mtx.Lock()
	c.ctor = nil
	c.rt = nil
	c.demoBfh.Release()
	c.demoFs.Release()
	c.mtx.Unlock()
	return nil
}

// _ is a type assertion
var (
	_ web_runtime.WebRuntimeController = ((*Controller)(nil))
	_ web_runtime.WebRuntimeHandler    = ((*Controller)(nil))
)
