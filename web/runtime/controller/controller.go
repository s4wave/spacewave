package web_runtime_controller

import (
	"context"
	"net/http"
	"strings"

	plugin "github.com/aperturerobotics/bldr/plugin"
	web_document "github.com/aperturerobotics/bldr/web/document"
	fetch "github.com/aperturerobotics/bldr/web/fetch"
	web_pkg_http "github.com/aperturerobotics/bldr/web/pkg/http"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	unixfs_access_http "github.com/aperturerobotics/hydra/unixfs/access/http"
	"github.com/aperturerobotics/util/broadcast"
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

	// pkgServer is the web pkg server
	pkgServer *web_pkg_http.Server

	// bcast guards below fields
	bcast broadcast.Broadcast
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

		// Pass false to wait for missing web pkgs instead of 404
		pkgServer: web_pkg_http.NewServer(le, bus, false),
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
	defer c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.rt == rt {
			c.rt = nil
			broadcast()
		}
	})

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.rt = rt
		broadcast()
	})

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
		var trig <-chan struct{}
		var rt web_runtime.WebRuntime
		c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			rt = c.rt
			if rt == nil {
				trig = getWaitCh()
			}
		})
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
	switch dir := di.GetDirective().(type) {
	case web_runtime.LookupWebRuntime:
		if dir.LookupWebRuntimeID() == c.runtimeID || dir.LookupWebRuntimeID() == "" {
			return directive.R(directive.NewGetterResolver[web_runtime.LookupWebRuntimeValue](c.GetWebRuntime), nil)
		}
	}
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
	if strings.HasPrefix(rpath, "/b/") {
		// /b/pkg/ is for Web module distribution files (like react)
		bPkgPrefix := plugin.PluginWebPkgHttpPrefix
		if strings.HasPrefix(rpath, bPkgPrefix) && len(rpath) > len(bPkgPrefix) {
			pkgPath := rpath[len(bPkgPrefix):]
			c.ServeWebModuleHTTP(pkgPath, rw, req)
			return
		}

		// /b/pd/ is for Web plugin distribution files
		c.le.Debugf("serve /b/ path: %s", rpath)
		bPdPrefix := plugin.PluginDistHttpPrefix
		if strings.HasPrefix(rpath, bPdPrefix) && len(rpath) > len(bPdPrefix) {
			pluginID, suffix, err := plugin.ParseHTTPPathPluginID(rpath[len(plugin.PluginDistHttpPrefix):])
			if err != nil {
				rw.WriteHeader(404)
				_, _ = rw.Write([]byte("bldr: invalid plugin id: " + err.Error()))
				return
			}

			req.URL.Path = suffix
			c.ServePluginDistFsHTTP(pluginID, rw, req)
			return
		}

		// /b/ps/ is for Web plugin entrypoints (plugin start)
		bPsPrefix := plugin.PluginStartHttpPrefix
		if strings.HasPrefix(rpath, bPsPrefix) && len(rpath) > len(bPsPrefix) {
			req.URL.Path = bPsPrefix[len(bPsPrefix):]
			c.ServePluginStartHTTP(rw, req)
			return
		}

		// other /b/ paths are not found
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("404 not found"))
		return
	}

	// /p/ is for plugin handlers
	// /p/{plugin-id}/... will be forwarded to the loaded plugin.
	if strings.HasPrefix(rpath, plugin.PluginHttpPrefix) {
		pluginID, suffix, err := plugin.ParseHTTPPathPluginID(rpath[len(plugin.PluginHttpPrefix):])
		if err != nil {
			rw.WriteHeader(404)
			_, _ = rw.Write([]byte("bldr: invalid plugin id: " + err.Error()))
			return
		}

		req.URL.Path = suffix
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
		_, _ = rw.Write([]byte("bldr: request failed: plugin " + pluginID + ": " + err.Error()))
		return
	}
}

// ServePluginDistFsHTTP serves a HTTP request for a plugin dist filesystem.
func (c *Controller) ServePluginDistFsHTTP(pluginID string, rw http.ResponseWriter, req *http.Request) {
	// access the fs with unixfs_access, return not found if the fs is not available
	ctx, ctxCancel := context.WithCancel(req.Context())
	defer ctxCancel()

	c.le.
		WithField("plugin-id", pluginID).
		WithField("path", req.URL.Path).
		Debug("accessing plugin dist filesystem")
	// see: plugin/host/controller/plugin-tracker.go distFsID
	unixFsID := plugin.PluginDistFsId + "/" + pluginID
	handlerBuilder := unixfs_access_http.NewHTTPHandlerBuilder(c.bus, unixFsID, "", "", true)
	handler, relHandler, err := handlerBuilder(ctx, ctxCancel)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("bldr: request failed: " + err.Error()))
		return
	}
	defer relHandler()

	(*handler).ServeHTTP(rw, req)
}

// ServePluginStartHTTP serves the /b/ps/ filesystem.
// This contains .js files for plugin startup on the web platform.
func (c *Controller) ServePluginStartHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "text/javascript")
	rw.WriteHeader(200)
	_, _ = rw.Write([]byte("console.log('TODO return plugin.js for worker!')\n"))
}

// ServeWebModuleHTTP serves a ServiceWorker HTTP request for a web module at /b/pkg.
//
// pkgPath is the path after /b/pkg/ - for example, "pkg" or "pkg/client.js" or "@my/pkg".
// The first element(s) of the path (split by /) are used as the package name.
// If the path begins with @, it is treated as a scope: @scope/package/...
func (c *Controller) ServeWebModuleHTTP(pkgPath string, rw http.ResponseWriter, req *http.Request) {
	c.pkgServer.ServeWebModuleHTTP(pkgPath, rw, req)
}

// HandleWebDocument handles an incoming WebDocument.
func (c *Controller) HandleWebDocument(wv web_document.WebDocument) {
	// no-op
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		c.rt = nil
		broadcast()
	})
	return nil
}

// _ is a type assertion
var (
	_ web_runtime.WebRuntimeController = ((*Controller)(nil))
	_ web_runtime.WebRuntimeHandler    = ((*Controller)(nil))
)
