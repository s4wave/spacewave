package web_fetch_service

import (
	"context"
	"net/http"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	web_fetch "github.com/aperturerobotics/bldr/web/fetch"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/fetch/service"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller fetches plugins via the PluginFetch service on a loaded plugin.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// handler is the srpc fetch service handler
	handler srpc.Handler
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	c := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
	c.handler = web_fetch.NewSRPCFetchServiceHandler(c, c.conf.GetServiceId())
	return c
}

// GetServiceID returns the ServiceID the controller will respond to.
func (c *Controller) GetServiceID() string {
	serviceID := c.conf.GetServiceId()
	if serviceID == "" {
		serviceID = web_fetch.SRPCFetchServiceServiceID
	}
	return serviceID
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"Fetch RPC service via LookupHTTPHandler directive",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == c.GetServiceID() {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
		}
	}
	return nil, nil
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != "" && serviceID != c.GetServiceID() {
		return false, nil
	}
	return c.handler.InvokeMethod(serviceID, methodID, strm)
}

// Fetch performs the fetch request with a stream.
func (c *Controller) Fetch(strm web_fetch.SRPCFetchService_FetchStream) error {
	return web_fetch.HandleFetch(strm, c.ServeHTTP)
}

// ServeHTTP serves HTTP for the Fetch controller.
func (c *Controller) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	handler, handlerRef, err := bifrost_http.ExLookupFirstHTTPHandler(ctx, c.bus, req.URL.String(), "", true)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	if handlerRef == nil {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("bldr: handler not found for url"))
		return
	}

	defer handlerRef.Release()
	handler.ServeHTTP(rw, req)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller            = ((*Controller)(nil))
	_ srpc.Invoker                     = ((*Controller)(nil))
	_ web_fetch.SRPCFetchServiceServer = ((*Controller)(nil))
)
