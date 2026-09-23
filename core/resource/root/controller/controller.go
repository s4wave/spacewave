package resource_root_controller

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_command "github.com/s4wave/spacewave/core/resource/command"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	resource_objecttype_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	resource_worldop_registry "github.com/s4wave/spacewave/core/resource/worldop/registry"
	space_world_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
)

// ControllerID is the controller id.
const ControllerID = "resource/root"

// Version is the component version
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "s4wave core resource root server controller"

// Controller is the root resource controller.
type Controller struct {
	*bus.BusController[*Config]
	// mux is the rpc mux for the service.
	mux srpc.Mux
	// rootResourceMux is the rpc mux for the root resource.
	rootResourceMux srpc.Mux
	// server is the rpc server
	server *resource_server.ResourceServer
	// rootResource is the root resource server
	rootResource *resource_root.CoreRootServer
	// httpPathPrefix locates this installation beneath its host plugin HTTP route.
	httpPathPrefix string
	// apps owns shared runtime attachments for explicitly supplied Storage.
	apps *keyed.KeyedRefCount[resource_root.AppStorage, *promise.Promise[*appRuntime]]
	// parent is the root of the installation hosting this nested app, if any.
	parent *Controller
	// hostPluginID is the plugin serving this installation, set before apps
	// start in Execute.
	hostPluginID string
	// registries are the plugin capability registries, shared with parent.
	registries *registries
	// commandsManager is the commands manager resource
	commandsManager *resource_command.CommandsManager

	// yieldBroker and listenerStatus are shared with the resource listener
	// controller through the composition root; injected into the root
	// resource after construction.
	yieldBroker    *yield_policy.Broker
	listenerStatus *resource_listener.StatusBroker
}

// Option configures the root resource controller factory.
type Option func(*Controller)

// WithYieldBroker injects the shared listener yield broker.
func WithYieldBroker(broker *yield_policy.Broker) Option {
	return func(c *Controller) { c.yieldBroker = broker }
}

// WithListenerStatusBroker injects the shared listener status broker.
func WithListenerStatusBroker(broker *resource_listener.StatusBroker) Option {
	return func(c *Controller) { c.listenerStatus = broker }
}

// withParent nests the controller in parent's installation: it serves the
// parent's plugin registries and Spaces load plugins through the parent host.
func withParent(parent *Controller) Option {
	return func(c *Controller) {
		c.parent = parent
		c.registries = parent.registries
	}
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus, opts ...Option) controller.Factory {
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
			c := &Controller{BusController: base}
			c.apps = c.newAppRegistry()
			for _, opt := range opts {
				opt(c)
			}

			// create the resource server
			c.rootResourceMux = srpc.NewMux()
			c.server = resource_server.NewResourceServer(c.rootResourceMux)

			// register the resource server to the mux
			serviceID := c.GetServiceID()
			c.mux = srpc.NewMux()
			if err := c.mux.Register(resource.NewSRPCResourceServiceHandler(c.server, serviceID)); err != nil {
				return nil, err
			}

			// create the root resource
			c.rootResource = resource_root.NewCoreRootServer(base.GetLogger(), b)
			c.rootResource.SetMountAppFunc(c.mountApp)
			if c.yieldBroker != nil {
				c.rootResource.SetYieldBroker(c.yieldBroker)
			}
			if c.listenerStatus != nil {
				c.rootResource.SetListenerStatusBroker(c.listenerStatus)
			}
			if err := c.rootResource.Register(c.rootResourceMux); err != nil {
				return nil, err
			}

			// register handler to the root resource mux
			if err := s4wave_root.SRPCRegisterRootResourceService(c.rootResourceMux, c.rootResource); err != nil {
				return nil, err
			}

			// serve the plugin capability registries
			if c.registries == nil {
				c.registries = newRegistries(base.GetLogger(), b)
			}
			if err := c.registries.register(c.rootResourceMux); err != nil {
				return nil, err
			}

			// create and register the commands manager on the root resource mux
			c.commandsManager = resource_command.NewCommandsManager()
			if err := s4wave_command_registry.SRPCRegisterCommandRegistryResourceService(c.rootResourceMux, c.commandsManager); err != nil {
				return nil, err
			}

			return c, nil
		},
	)
}

// Execute registers child controllers for the root resource lifecycle.
func (c *Controller) Execute(ctx context.Context) error {
	if c.parent != nil {
		c.hostPluginID = c.parent.hostPluginID
	} else if info := bldr_plugin.GetPluginContextInfo(ctx); info != nil {
		c.hostPluginID = info.GetPluginMeta().GetPluginId()
	}
	c.rootResource.SetHostPluginID(c.hostPluginID)
	c.apps.SetContext(ctx, true)
	defer c.apps.ClearContext()
	b := c.GetBus()
	le := c.GetLogger()
	var releases []func()
	releaseAll := func() {
		for _, v := range slices.Backward(releases) {
			v()
		}
		releases = nil
	}

	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	objectTypeRel, err := b.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		return err
	}
	releases = append(releases, objectTypeRel)

	bridgeCtrl := resource_objecttype_registry.NewBridgeController(le, b, c.registries.objectType)
	bridgeRel, err := b.AddController(ctx, bridgeCtrl, nil)
	if err != nil {
		releaseAll()
		return err
	}
	releases = append(releases, bridgeRel)

	worldOpBridgeCtrl := resource_worldop_registry.NewWorldOpRegistryBridgeController(le, b, c.registries.worldOp)
	worldOpBridgeRel, err := b.AddController(ctx, worldOpBridgeCtrl, nil)
	if err != nil {
		releaseAll()
		return err
	}
	releases = append(releases, worldOpBridgeRel)
	defer releaseAll()

	<-ctx.Done()
	return nil
}

// Close releases permanent root resources after controller execution stops.
func (c *Controller) Close() error {
	c.rootResource.Close()
	return c.BusController.Close()
}

// GetServiceID returns the ServiceID the controller will respond to.
func (c *Controller) GetServiceID() string {
	serviceID := c.GetConfig().GetServiceId()
	if serviceID == "" {
		serviceID = resource.SRPCResourceServiceServiceID
	}
	return serviceID
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	return c.mux.InvokeMethod(serviceID, methodID, strm)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case bifrost_rpc.LookupRpcService:
		serviceID := d.LookupRpcServiceID()
		if serviceID == c.GetServiceID() {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
		}
	}

	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
