// Package web_runtime_http serves the Bldr /b/ routes from a bus.
package web_runtime_http

import (
	"context"
	"net/http"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	fetch "github.com/s4wave/spacewave/bldr/web/fetch"
	web_pkg_http "github.com/s4wave/spacewave/bldr/web/pkg/http"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// Handler serves frontend modules, web packages, and plugin files from a bus.
type Handler struct {
	// le is the logger.
	le *logrus.Entry
	// b is the bus serving frontend services and plugin filesystems.
	b bus.Bus
	// pkgServer serves web packages.
	pkgServer *web_pkg_http.Server
}

// NewHandler constructs a Handler for the bus.
func NewHandler(le *logrus.Entry, b bus.Bus) *Handler {
	return &Handler{
		le:        le,
		b:         b,
		pkgServer: web_pkg_http.NewServer(le, b, false),
	}
}

// ServeBldrHTTP serves req when its path is a frontend module, web package,
// plugin distribution, or plugin asset route, and reports whether it did.
func (h *Handler) ServeBldrHTTP(rw http.ResponseWriter, req *http.Request) bool {
	// Route by the Bldr path prefix.
	rpath := req.URL.Path

	// /b/fe/ routes to a frontend service. Modules use an existing compiler
	// grant; expired attachments settle instead of waiting for a service that
	// will never be recreated.
	if strings.HasPrefix(rpath, "/b/fe/") {
		h.serveFrontend(rw, req)
		return true
	}

	// /b/pkg/ serves web module distribution files such as react.
	if pkgPath, ok := cutRoute(rpath, bldr_plugin.PluginWebPkgHttpPrefix); ok {
		h.pkgServer.ServeWebModuleHTTP(pkgPath, rw, req)
		return true
	}

	// /b/pd/ and /b/pa/ serve plugin distribution and asset files.
	if artifactPath, ok := cutRoute(rpath, bldr_plugin.PluginDistHttpPrefix); ok {
		h.servePluginFs(rw, req, artifactPath, bldr_plugin.PluginDistFsId)
		return true
	}
	if artifactPath, ok := cutRoute(rpath, bldr_plugin.PluginAssetsHttpPrefix); ok {
		h.servePluginFs(rw, req, artifactPath, bldr_plugin.PluginAssetsFsId)
		return true
	}
	return false
}

// serveFrontend proxies a frontend module request to its frontend service.
func (h *Handler) serveFrontend(rw http.ResponseWriter, req *http.Request) {
	// Resolve the frontend service that owns the module path.
	setNoCacheHeaders(rw.Header())
	serviceID, err := frontend.RouteService(req.URL.Path)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// Fetch the module through the service; RPC routes do not wait for it.
	wait := !strings.HasPrefix(req.URL.Path, "/b/fe/rpc/")
	client := frontend.NewSRPCFrontendClientWithServiceID(bifrost_rpc.NewBusClientWithWait(h.b, wait), serviceID)
	err = fetch.Fetch(req.Context(), func(ctx context.Context) (fetch.SRPCFetchService_FetchClient, error) {
		return client.Fetch(ctx)
	}, req, rw)
	if err != nil && req.Context().Err() == nil {
		h.le.WithError(err).Warn("frontend module request failed")
		http.Error(rw, "frontend module unavailable", http.StatusBadGateway)
	}
}

// servePluginFs serves a file from the plugin UnixFS that fsID names.
func (h *Handler) servePluginFs(
	rw http.ResponseWriter,
	req *http.Request,
	artifactPath string,
	fsID func(pluginID string) string,
) {
	// Split the plugin ID from the file path.
	pluginID, suffix, err := bldr_plugin.ParseHTTPPathPluginArtifact(artifactPath)
	if err != nil {
		http.Error(rw, "bldr: invalid plugin id: "+err.Error(), http.StatusNotFound)
		return
	}
	h.le.
		WithField("plugin-id", pluginID).
		WithField("path", suffix).
		Debug("accessing plugin filesystem")

	// see: plugin/host/controller/plugin-tracker.go distFsID and assetsFsID
	handler := unixfs_access_http.NewHTTPHandler(req.Context(), h.b, fsID(pluginID), "", "", true)
	setNoCacheHeaders(rw.Header())
	req.URL.Path = suffix
	handler.ServeHTTP(rw, req)
}

// cutRoute returns the path after prefix when path continues past it.
func cutRoute(path, prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(path, prefix)
	return rest, ok && rest != ""
}

// setNoCacheHeaders keeps handler headers non-load-bearing for plugin asset
// freshness; a generation-scoped ServiceWorker cache handles warm reads.
func setNoCacheHeaders(h http.Header) {
	h.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}
