package web_runtime_controller

import (
	"context"
	"net/http"
	"strings"
	"sync"

	plugin "github.com/aperturerobotics/bldr/plugin"
	web_document "github.com/aperturerobotics/bldr/web/document"
	fetch "github.com/aperturerobotics/bldr/web/fetch"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a runtime with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler web_runtime.WebRuntimeHandler,
) (web_runtime.WebRuntime, error)

// Controller implements a common bldr web runtime controller.
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
		// _ = rt.Close(ctx)
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
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// HandleFetch handles an incoming Fetch request from the web runtime.
// The Client ID can be used to distinguish between windows / browser tabs.
func (c *Controller) HandleFetch(strm fetch.SRPCFetchService_FetchStream) error {
	return fetch.HandleFetch(strm, c.ServeServiceWorkerHTTP)
}

// ServeServiceWorkerHTTP serves a ServiceWorker HTTP request.
func (c *Controller) ServeServiceWorkerHTTP(rw http.ResponseWriter, req *http.Request) {
	rurl := req.URL
	rpath := rurl.Path

	// /b/ is for bldr internals
	// /b/dist/ is for Web plugin distribution files
	if strings.HasPrefix(rpath, "/b/") {
		rw.WriteHeader(200)
		_, _ = rw.Write([]byte("TODO serve /b/ path: " + rpath))
		return
	}

	// /p/ is for plugin handlers
	// /p/{plugin-id}/... will be forwarded to the loaded plugin.
	if strings.HasPrefix(rpath, plugin.PluginHttpPrefix) {
		ppath := rpath[3:]
		slashIdx := strings.IndexRune(ppath, '/')
		pluginID := ppath
		if slashIdx != -1 {
			pluginID = ppath[:slashIdx]
		}

		if err := plugin.ValidatePluginID(pluginID, false); err != nil {
			rw.WriteHeader(404)
			_, _ = rw.Write([]byte("bldr: invalid plugin id: " + err.Error()))
			return
		}

		req.URL.Path = ppath[slashIdx:]
		c.ServePluginHTTP(pluginID, rw, req)
		return
	}

	rw.WriteHeader(404)
	_, _ = rw.Write([]byte("bldr: unhandled path: " + rpath))
}

// ServePluginHTTP serves a ServiceWorker HTTP request for a plugin.
func (c *Controller) ServePluginHTTP(pluginID string, rw http.ResponseWriter, req *http.Request) {
	// call LoadPlugin to get a handle to the desired plugin.
	ctx := req.Context()
	c.le.
		WithField("plugin-id", pluginID).
		WithField("path", req.URL.Path).
		Debug("forwarding http call to plugin")
	rpcClient, rpcClientRef, err := plugin.ExPluginLoadWaitClient(ctx, c.bus, pluginID, nil)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("bldr: load plugin failed: " + pluginID + ": " + err.Error()))
		return
	}
	if rpcClient == nil {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("bldr: plugin not found: " + pluginID))
		return
	}
	defer rpcClientRef.Release()

	fetchClient := fetch.NewSRPCFetchServiceClient(rpcClient)
	err = fetch.Fetch(ctx, fetchClient.Fetch, req, rw)
	if err != nil && err != context.Canceled {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("bldr: request failed: " + pluginID + ": " + err.Error()))
		return
	}
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
	c.mtx.Unlock()
	return nil
}

// _ is a type assertion
var (
	_ web_runtime.WebRuntimeController = ((*Controller)(nil))
	_ web_runtime.WebRuntimeHandler    = ((*Controller)(nil))
)
