package web_runtime_controller

import (
	"context"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	fetch "github.com/s4wave/spacewave/bldr/web/fetch"
	web_pkg_http "github.com/s4wave/spacewave/bldr/web/pkg/http"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
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
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor

	// runtimeID is the controller id to use
	runtimeID string
	// runtimeVersion is the version
	runtimeVersion controller.Version

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
	runtimeVersion controller.Version,
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
		if dir.LookupWebRuntimeID() == c.runtimeID {
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
				http.Error(rw, "bldr: invalid plugin id: "+err.Error(), http.StatusNotFound)
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
				http.Error(rw, "bldr: invalid plugin id: "+err.Error(), http.StatusNotFound)
				return
			}

			req.URL.Path = suffix
			c.ServePluginAssetsFsHTTP(pluginID, rw, req)
			return
		}

		if rpath == "/b/__index.html" {
			c.ServeBrowserIndexHTML(rw, req)
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
			http.Error(rw, "bldr: invalid plugin id: "+err.Error(), http.StatusNotFound)
			return
		}

		req.URL.Path = suffix
		c.ServePluginHTTP(pluginID, rw, req)
		return
	}

	http.Error(rw, "bldr: unhandled path: "+rpath, http.StatusNotFound)
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
		http.Error(rw, "bldr: load plugin failed: "+pluginID+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	if rpcClient == nil {
		http.Error(rw, "bldr: plugin not found: "+pluginID, http.StatusNotFound)
		return
	}
	defer rpcClientRef.Release()

	fetchClient := fetch.NewSRPCFetchServiceClient(rpcClient)
	err = fetch.Fetch(ctx, fetchClient.Fetch, req, rw)
	if err != nil && err != context.Canceled {
		http.Error(rw, "bldr: request failed: plugin "+pluginID+": "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// setNoCacheHeaders sets headers that prevent client-side caching.
func setNoCacheHeaders(hdr http.Header) {
	hdr.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	hdr.Set("Pragma", "no-cache")
	hdr.Set("Expires", "0")
}

const immutableContentCacheControl = "public, max-age=31536000, immutable"

// setPluginFileCacheHeaders sets immutable cache headers for content-hashed plugin files.
func setPluginFileCacheHeaders(hdr http.Header, requestPath string) {
	if isContentHashedFilename(requestPath) {
		hdr.Set("Cache-Control", immutableContentCacheControl)
		hdr.Del("Pragma")
		hdr.Del("Expires")
		return
	}

	setNoCacheHeaders(hdr)
}

// isContentHashedFilename reports whether the request basename names immutable content.
//
// Bundler-hashed names keep the bytes stable for the path; a rebuild changes
// the filename, not the content served at the old filename.
func isContentHashedFilename(requestPath string) bool {
	if requestPath == "" || strings.ContainsAny(requestPath, "?#") {
		return false
	}

	name := path.Base(requestPath)
	if name == "." {
		return false
	}

	dash := strings.LastIndexByte(name, '-')
	if dash < 0 || dash == len(name)-1 {
		return false
	}

	dot := strings.IndexByte(name[dash+1:], '.')
	if dot < 0 {
		return false
	}

	hash := name[dash+1 : dash+1+dot]
	if len(hash) < 8 {
		return false
	}

	hasDigit := false
	hasUpper := false
	for _, ch := range hash {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case ch == '_':
		default:
			return false
		}
	}

	return hasDigit && hasUpper
}

// ServePluginDistFsHTTP serves a HTTP request for a plugin dist filesystem.
func (c *Controller) ServePluginDistFsHTTP(pluginID string, rw http.ResponseWriter, req *http.Request) {
	c.le.
		WithField("plugin-id", pluginID).
		WithField("path", req.URL.Path).
		Debug("accessing plugin dist filesystem")
	// see: plugin/host/controller/plugin-tracker.go distFsID
	unixFsID := bldr_plugin.PluginDistFsId(pluginID)
	handler := unixfs_access_http.NewHTTPHandler(req.Context(), c.bus, unixFsID, "", "", true)

	setPluginFileCacheHeaders(rw.Header(), req.URL.RequestURI())

	handler.ServeHTTP(rw, req)
}

// ServePluginAssetsFsHTTP serves a HTTP request for a plugin assets filesystem.
func (c *Controller) ServePluginAssetsFsHTTP(pluginID string, rw http.ResponseWriter, req *http.Request) {
	c.le.
		WithField("plugin-id", pluginID).
		WithField("path", req.URL.Path).
		Debug("accessing plugin assets filesystem")
	// see: plugin/host/controller/plugin-tracker.go assetsFsID
	unixFsID := bldr_plugin.PluginAssetsFsId(pluginID)
	handler := unixfs_access_http.NewHTTPHandler(req.Context(), c.bus, unixFsID, "", "", true)

	setPluginFileCacheHeaders(rw.Header(), req.URL.RequestURI())

	handler.ServeHTTP(rw, req)
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

// ServeBrowserIndexHTML serves the browser root document from the Go runtime.
func (c *Controller) ServeBrowserIndexHTML(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	indexHTML, err := web_entrypoint_index.RenderIndexHTML(web_entrypoint_index.IndexData{
		ImportMap:      web_entrypoint_index.ImportMap{Imports: map[string]string{}},
		EntrypointPath: "/boot.mjs",
	})
	if err != nil {
		http.Error(rw, "bldr: render browser index failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Content-Length", strconv.Itoa(len(indexHTML)))
	rw.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		if _, err := rw.Write([]byte(indexHTML)); err != nil && c.le != nil {
			c.le.WithError(err).Debug("write browser index response failed")
		}
	}
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
