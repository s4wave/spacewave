// Package download serves projected space file content via HTTP.
// URL: /p/spacewave-core/fs/u/{idx}/so/{soId}/...
// The /p/spacewave-core/ prefix is stripped by the web runtime.
// This handler receives: /fs/u/{idx}/so/{soId}/...
package space_http_download

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	space_http_header "github.com/s4wave/spacewave/core/space/http/header"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	space_unixfs "github.com/s4wave/spacewave/core/space/unixfs"
	unixfs_access_http "github.com/s4wave/spacewave/db/unixfs/access/http"
	"github.com/s4wave/spacewave/db/world"
	bifrost_http "github.com/s4wave/spacewave/net/http"
)

// ControllerID is the controller identifier.
const ControllerID = "space/http/download"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "serves unixfs file downloads via http"

// fsPathPrefix is the URL path prefix this controller handles.
const fsPathPrefix = "/fs/"

// Controller serves UnixFS file content for download via LookupHTTPHandler.
type Controller struct {
	*bus.BusController[*Config]
}

// NewFactory constructs the controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_http.LookupHTTPHandler:
		u := d.LookupHTTPHandlerURL()
		if u != nil && strings.HasPrefix(u.Path, fsPathPrefix) {
			return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(c), nil)
		}
	}
	return nil, nil
}

// downloadRequest holds parsed parameters from a download URL.
type downloadRequest struct {
	sessionIdx     uint32
	sharedObjectID string
	projectedPath  string
}

// parseDownloadURL parses /fs/u/{idx}/so/{soId}/...
func parseDownloadURL(path string) (*downloadRequest, error) {
	rest := strings.TrimPrefix(path, fsPathPrefix)
	projected, err := space_unixfs.ParseProjectedPath(rest)
	if err != nil {
		return nil, err
	}

	return &downloadRequest{
		sessionIdx:     projected.SessionIdx,
		sharedObjectID: projected.SharedObjectID,
		projectedPath:  projected.Path,
	}, nil
}

// ServeHTTP handles file download requests.
// /fs/u/{idx}/so/{soId}/...
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	le := c.GetLogger()
	b := c.GetBus()
	ctx := r.Context()

	req, err := parseDownloadURL(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid download URL: "+err.Error(), http.StatusBadRequest)
		return
	}

	resolved, cleanup, err := space_resolve.ResolveSpace(ctx, b, req.sessionIdx, req.sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to resolve space for download")
		http.Error(w, "space resolution failed", http.StatusServiceUnavailable)
		return
	}
	defer cleanup()

	ws := world.NewEngineWorldState(resolved.Engine, false)

	fsh, err := space_unixfs.BuildFSHandle(le, ws, req.sessionIdx, req.sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to create fs handle")
		http.Error(w, "failed to open filesystem", http.StatusInternalServerError)
		return
	}
	defer fsh.Release()

	// Build an http.FileSystem from the FSHandle.
	hfs := unixfs_access_http.NewFileSystem(ctx, fsh, "", "")

	// Rewrite the request URL to serve the specific file.
	nr := r.Clone(ctx)
	nr.URL = &url.URL{Path: "/" + req.projectedPath}

	// Set Content-Disposition unless inline mode is requested.
	if r.URL.Query().Get("inline") == "" {
		fileName := req.projectedPath
		if idx := strings.LastIndex(req.projectedPath, "/"); idx >= 0 {
			fileName = req.projectedPath[idx+1:]
		}
		space_http_header.SetAttachmentHeader(w, fileName)
	}

	// Serve the file using the standard http.FileServer.
	handler := unixfs_access_http.NewFileServer(hfs)
	handler.ServeHTTP(w, nr)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
