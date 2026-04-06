package web_runtime_controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_wazero_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	web_document "github.com/aperturerobotics/bldr/web/document"
	fetch "github.com/aperturerobotics/bldr/web/fetch"
	web_pkg_http "github.com/aperturerobotics/bldr/web/pkg/http"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	quickjs_wasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
	unixfs_access_http "github.com/aperturerobotics/hydra/unixfs/access/http"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/blang/semver/v4"
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
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case web_runtime.LookupWebRuntime:
		if dir.LookupWebRuntimeID() == c.runtimeID || dir.LookupWebRuntimeID() == "" {
			return directive.R(directive.NewGetterResolver(c.GetWebRuntime), nil)
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
		// COEP + CORP headers are required for resources loaded under
		// Cross-Origin-Embedder-Policy: require-corp. Module worker scripts
		// need COEP on their own response (the worker is a separate execution
		// context). CORP marks the resource as same-origin accessible.
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		rw.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		c.le.Debugf("serve /b/ path: %s", rpath)

		// /b/pkg/ is for Web module distribution files (like react)
		bPkgPrefix := bldr_plugin.PluginWebPkgHttpPrefix
		if strings.HasPrefix(rpath, bPkgPrefix) && len(rpath) > len(bPkgPrefix) {
			pkgPath := rpath[len(bPkgPrefix):]
			c.ServeWebModuleHTTP(pkgPath, rw, req)
			return
		}

		// /b/pd/ is for Web plugin distribution files
		bPdPrefix := bldr_plugin.PluginDistHttpPrefix
		if strings.HasPrefix(rpath, bPdPrefix) && len(rpath) > len(bPdPrefix) {
			pluginID, suffix, err := bldr_plugin.ParseHTTPPathPluginID(rpath[len(bldr_plugin.PluginDistHttpPrefix):])
			if err != nil {
				rw.WriteHeader(404)
				_, _ = rw.Write([]byte("bldr: invalid plugin id: " + err.Error()))
				return
			}

			req.URL.Path = suffix
			c.ServePluginDistFsHTTP(pluginID, rw, req)
			return
		}

		// /b/pa/ is for Web plugin distribution files
		bPaPrefix := bldr_plugin.PluginAssetsHttpPrefix
		if strings.HasPrefix(rpath, bPaPrefix) && len(rpath) > len(bPaPrefix) {
			pluginID, suffix, err := bldr_plugin.ParseHTTPPathPluginID(rpath[len(bldr_plugin.PluginAssetsHttpPrefix):])
			if err != nil {
				rw.WriteHeader(404)
				_, _ = rw.Write([]byte("bldr: invalid plugin id: " + err.Error()))
				return
			}

			req.URL.Path = suffix
			c.ServePluginAssetsFsHTTP(pluginID, rw, req)
			return
		}

		// /b/qjs/ is for QuickJS runtime files (WASM binary and boot harness)
		bQjsPrefix := bldr_plugin.QuickJSHttpPrefix
		if strings.HasPrefix(rpath, bQjsPrefix) && len(rpath) > len(bQjsPrefix) {
			filePath := rpath[len(bQjsPrefix):]
			c.ServeQuickJSHTTP(filePath, rw, req)
			return
		}

		// other /b/ paths are not found
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("404 not found"))
		return
	}

	// /p/ is for plugin handlers
	// /p/{plugin-id}/... will be forwarded to the loaded plugin.
	if strings.HasPrefix(rpath, bldr_plugin.PluginHttpPrefix) {
		pluginID, suffix, err := bldr_plugin.ParseHTTPPathPluginID(rpath[len(bldr_plugin.PluginHttpPrefix):])
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
	rpcClient, rpcClientRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, c.bus, pluginID, nil)
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

// setNoCacheHeaders sets headers that prevent client-side caching.
func setNoCacheHeaders(hdr http.Header) {
	hdr.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	hdr.Set("Pragma", "no-cache")
	hdr.Set("Expires", "0")
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
	unixFsID := bldr_plugin.PluginDistFsId(pluginID)
	handlerBuilder := unixfs_access_http.NewHTTPHandlerBuilder(c.bus, unixFsID, "", "", true)
	handler, relHandler, err := handlerBuilder(ctx, ctxCancel)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("bldr: request failed: " + err.Error()))
		return
	}
	defer relHandler()

	// set headers preventing caching
	// TODO: tell ServiceWorker to flush cache when plugin is updated!
	setNoCacheHeaders(rw.Header())

	handler.ServeHTTP(rw, req)
}

// ServePluginAssetsFsHTTP serves a HTTP request for a plugin assets filesystem.
func (c *Controller) ServePluginAssetsFsHTTP(pluginID string, rw http.ResponseWriter, req *http.Request) {
	// access the fs with unixfs_access, return not found if the fs is not available
	ctx, ctxCancel := context.WithCancel(req.Context())
	defer ctxCancel()

	c.le.
		WithField("plugin-id", pluginID).
		WithField("path", req.URL.Path).
		Debug("accessing plugin assets filesystem")
	// see: plugin/host/controller/plugin-tracker.go assetsFsID
	unixFsID := bldr_plugin.PluginAssetsFsId(pluginID)
	handlerBuilder := unixfs_access_http.NewHTTPHandlerBuilder(c.bus, unixFsID, "", "", true)
	handler, relHandler, err := handlerBuilder(ctx, ctxCancel)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("bldr: request failed: " + err.Error()))
		return
	}
	defer relHandler()

	// Content-hashed filenames change on every rebuild so the old path
	// becomes a 404 if cached.  Disable caching to match /b/pd/ behavior.
	// TODO: use immutable cache headers keyed on content hash instead.
	setNoCacheHeaders(rw.Header())

	handler.ServeHTTP(rw, req)
}

// ServeQuickJSHTTP serves QuickJS runtime files at /b/qjs/.
// Available files:
//   - qjs-wasi.wasm - The QuickJS WASI binary
//   - plugin-quickjs.esm.js - The boot harness for running plugins
func (c *Controller) ServeQuickJSHTTP(filePath string, rw http.ResponseWriter, req *http.Request) {
	c.le.
		WithField("path", filePath).
		Debug("serving QuickJS file")

	var content []byte
	var contentType string

	switch filePath {
	case "qjs-wasi.wasm":
		content = quickjs_wasi.QuickJSWASM
		contentType = "application/wasm"
	case "plugin-quickjs.esm.js":
		var err error
		content, err = plugin_host_wazero_quickjs.PluginQuickjsBoot.ReadFile("plugin-quickjs.esm.js")
		if err != nil {
			rw.WriteHeader(500)
			_, _ = rw.Write([]byte("bldr: failed to read boot harness: " + err.Error()))
			return
		}
		contentType = "application/javascript"
	default:
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("bldr: unknown QuickJS file: " + filePath))
		return
	}

	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Content-Length", strconv.Itoa(len(content)))
	rw.WriteHeader(200)
	_, _ = rw.Write(content)
}

// ServeWebModuleHTTP serves a ServiceWorker HTTP request for a web module at /b/pkg.
//
// pkgPath is the path after /b/pkg/ - for example, "pkg" or "pkg/client.js" or "@my/pkg".
// The first element(s) of the path (split by /) are used as the package name.
// If the path begins with @, it is treated as a scope: @scope/package/...
func (c *Controller) ServeWebModuleHTTP(pkgPath string, rw http.ResponseWriter, req *http.Request) {
	// set headers preventing caching
	// we always want to do this since WebModule might be loaded from an alternative source

	// TODO: This causes an issue where the request never loads. Why?
	// setNoCacheHeaders(rw.Header())

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
