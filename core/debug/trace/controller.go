// Package debug_trace serves Go runtime trace and pprof data via HTTP.
//
// GET /p/spacewave-core/debugz/trace starts a trace, captures for the
// configured duration (default 30s), then returns the trace data.
//
// GET /p/spacewave-core/debugz/pprof/... serves pprof profile data.
// The /p/spacewave-core/ prefix is stripped by the web runtime.
//
// This handler receives: /debugz/...
package debug_trace

import (
	"bytes"
	"context"
	"net/http"
	http_pprof "net/http/pprof"
	runtime_trace "runtime/trace"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	bifrost_http "github.com/s4wave/spacewave/net/http"
)

// ControllerID is the controller identifier.
const ControllerID = "debug/trace"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
const controllerDescrip = "debug runtime trace and pprof http endpoint"

const (
	// tracePathPrefix is the URL path prefix for trace requests.
	tracePathPrefix = "/debugz/trace"
	// pprofPathPrefix is the URL path prefix for pprof requests.
	pprofPathPrefix = "/debugz/pprof"
)

// defaultDuration is the default trace capture duration.
const defaultDuration = 30 * time.Second

// Controller serves runtime trace captures via LookupHTTPHandler.
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
		if u != nil && (hasDebugPrefix(u.Path, tracePathPrefix) || hasDebugPrefix(u.Path, pprofPathPrefix)) {
			return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(c), nil)
		}
	}
	return nil, nil
}

// ServeHTTP handles debug trace and pprof requests.
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case hasDebugPrefix(r.URL.Path, pprofPathPrefix):
		c.servePprof(w, r)
		return
	case hasDebugPrefix(r.URL.Path, tracePathPrefix):
		c.serveTrace(w, r)
		return
	default:
		http.NotFound(w, r)
		return
	}
}

// serveTrace starts a runtime trace, captures for the configured duration, and
// writes the trace data to the response as a downloadable file.
func (c *Controller) serveTrace(w http.ResponseWriter, r *http.Request) {
	le := c.GetLogger()

	dur := defaultDuration
	if d := c.GetConfig().GetTraceDurationSeconds(); d > 0 {
		dur = time.Duration(d) * time.Second
	}

	// Parse optional ?seconds=N query parameter.
	if s := r.URL.Query().Get("seconds"); s != "" {
		if parsed, err := time.ParseDuration(s + "s"); err == nil && parsed > 0 {
			dur = parsed
		}
	}

	le.WithField("duration", dur).Info("starting runtime trace capture")

	var buf bytes.Buffer
	if err := runtime_trace.Start(&buf); err != nil {
		http.Error(w, "trace already active: "+err.Error(), http.StatusConflict)
		return
	}

	ctx := r.Context()
	select {
	case <-time.After(dur):
	case <-ctx.Done():
	}
	runtime_trace.Stop()

	le.WithField("bytes", buf.Len()).Info("trace capture complete")

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\"trace.out\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// servePprof serves pprof index pages and named profile endpoints.
func (c *Controller) servePprof(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == pprofPathPrefix {
		http.Redirect(w, r, pprofPathPrefix+"/", http.StatusMovedPermanently)
		return
	}

	rewrittenReq := rewritePprofRequest(r)
	switch pprofPath := strings.TrimPrefix(path, pprofPathPrefix); pprofPath {
	case "/":
		http_pprof.Index(w, rewrittenReq)
	case "/cmdline":
		http_pprof.Cmdline(w, rewrittenReq)
	case "/profile":
		http_pprof.Profile(w, rewrittenReq)
	case "/symbol":
		http_pprof.Symbol(w, rewrittenReq)
	case "/trace":
		http_pprof.Trace(w, rewrittenReq)
	default:
		name := strings.TrimPrefix(pprofPath, "/")
		if name == "" || strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		http_pprof.Handler(name).ServeHTTP(w, rewrittenReq)
	}
}

func rewritePprofRequest(r *http.Request) *http.Request {
	rewrittenReq := r.Clone(r.Context())
	rewrittenURL := *r.URL
	pprofPath := strings.TrimPrefix(r.URL.Path, pprofPathPrefix)
	if pprofPath == "" {
		pprofPath = "/"
	}
	rewrittenURL.Path = "/debug/pprof" + pprofPath
	rewrittenReq.URL = &rewrittenURL
	return rewrittenReq
}

func hasDebugPrefix(path string, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
