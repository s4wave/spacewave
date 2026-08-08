// Package export serves projected space data via HTTP zip routes.
// URL: /p/spacewave-core/export/u/{idx}/so/{soId}/...
// URL: /p/spacewave-core/export-batch/{base-path}/{b64}/{filename}.zip
// The /p/spacewave-core/ prefix is stripped by the web runtime.
package space_http_export

import (
	"context"
	"net/http"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	space_http_header "github.com/s4wave/spacewave/core/space/http/header"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	space_unixfs "github.com/s4wave/spacewave/core/space/unixfs"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	bifrost_http "github.com/s4wave/spacewave/net/http"
)

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "serves projected space zip exports via http"

// Controller serves projected space zip archives for download via LookupHTTPHandler.
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

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_http.LookupHTTPHandler:
		u := d.LookupHTTPHandlerURL()
		if u != nil && (strings.HasPrefix(u.Path, exportPathPrefix) || strings.HasPrefix(u.Path, exportBatchPathPrefix)) {
			return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(c), nil)
		}
	}
	return nil, nil
}

// ServeHTTP parses projected export paths and dispatches zip export.
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, exportBatchPathPrefix) {
		c.serveBatchExport(w, r)
		return
	}
	c.serveProjectedExport(w, r)
}

func (c *Controller) serveProjectedExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	le := c.GetLogger()
	b := c.GetBus()

	req, err := parseExportURL(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid export URL: "+err.Error(), http.StatusBadRequest)
		return
	}

	resolved, cleanup, err := space_resolve.ResolveSpace(ctx, b, req.sessionIdx, req.sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to resolve space for export")
		http.Error(w, "space resolution failed", http.StatusServiceUnavailable)
		return
	}
	defer cleanup()

	ws := world.NewEngineWorldState(resolved.Engine, false)
	rootHandle, err := space_unixfs.BuildFSHandle(le, ws, req.sessionIdx, req.sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to build projected fs handle")
		http.Error(w, "failed to open projected filesystem", http.StatusInternalServerError)
		return
	}
	defer rootHandle.Release()

	lookupPath, zipRoot := resolveProjectedExportTarget(req)
	targetHandle, _, err := rootHandle.LookupPath(ctx, lookupPath)
	if err != nil {
		http.Error(w, "export path not found", http.StatusNotFound)
		return
	}
	defer targetHandle.Release()

	w.Header().Set("Content-Type", "application/zip")
	space_http_header.SetAttachmentHeader(w, buildExportFilename(req.projectedPath))
	if err := streamProjectedExport(ctx, w, targetHandle, zipRoot); err != nil {
		le.WithError(err).Warn("failed to stream projected export")
	}
}

func (c *Controller) serveBatchExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	le := c.GetLogger()
	b := c.GetBus()

	req, err := parseBatchExportURL(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid export-batch URL: "+err.Error(), http.StatusBadRequest)
		return
	}

	resolved, cleanup, err := space_resolve.ResolveSpace(ctx, b, req.sessionIdx, req.sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to resolve space for batch export")
		http.Error(w, "space resolution failed", http.StatusServiceUnavailable)
		return
	}
	defer cleanup()

	ws := world.NewEngineWorldState(resolved.Engine, false)
	rootHandle, err := space_unixfs.BuildFSHandle(le, ws, req.sessionIdx, req.sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to build projected fs handle")
		http.Error(w, "failed to open projected filesystem", http.StatusInternalServerError)
		return
	}
	defer rootHandle.Release()

	baseHandle, _, err := rootHandle.LookupPath(ctx, req.basePath)
	if err != nil {
		http.Error(w, "batch export base path not found", http.StatusNotFound)
		return
	}
	defer baseHandle.Release()

	w.Header().Set("Content-Type", "application/zip")
	space_http_header.SetAttachmentHeader(w, req.filename)
	if err := exportBatchZip(ctx, w, baseHandle, req.paths); err != nil {
		le.WithError(err).Warn("failed to stream batch export")
	}
}

func streamProjectedExport(ctx context.Context, w http.ResponseWriter, targetHandle *unixfs.FSHandle, zipRoot string) error {
	if zipRoot == "" {
		return exportZip(ctx, w, targetHandle)
	}
	return exportNamedZip(ctx, w, targetHandle, zipRoot)
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
