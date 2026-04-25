package resource_root_controller

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_command "github.com/s4wave/spacewave/core/resource/command"
	resource_configtype_registry "github.com/s4wave/spacewave/core/resource/configtype/registry"
	resource_objecttype_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	resource_viewer_registry "github.com/s4wave/spacewave/core/resource/viewer/registry"
	resource_worldop_registry "github.com/s4wave/spacewave/core/resource/worldop/registry"
	space_world_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// ControllerID is the controller id.
const ControllerID = "resource/root"

// Version is the component version
var Version = semver.MustParse("0.0.1")

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
	// viewerRegistry is the viewer registry resource
	viewerRegistry *resource_viewer_registry.ViewerRegistryResource
	// objectTypeRegistry is the ObjectType registry resource
	objectTypeRegistry *resource_objecttype_registry.ObjectTypeRegistryResource
	// worldOpRegistry is the WorldOp registry resource
	worldOpRegistry *resource_worldop_registry.WorldOpRegistryResource
	// configTypeRegistry is the ConfigType registry resource
	configTypeRegistry *resource_configtype_registry.ConfigTypeRegistryResource
	// commandsManager is the commands manager resource
	commandsManager *resource_command.CommandsManager
}

// NewFactory constructs the component factory.
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
			c := &Controller{BusController: base}

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
			if err := c.rootResource.Register(c.rootResourceMux); err != nil {
				return nil, err
			}

			// register handler to the root resource mux
			if err := s4wave_root.SRPCRegisterRootResourceService(c.rootResourceMux, c.rootResource); err != nil {
				return nil, err
			}

			// create and register the viewer registry on the root resource mux
			c.viewerRegistry = resource_viewer_registry.NewViewerRegistryResource()
			if err := s4wave_viewer_registry.SRPCRegisterViewerRegistryResourceService(c.rootResourceMux, c.viewerRegistry); err != nil {
				return nil, err
			}

			// create and register the ObjectType registry on the root resource mux
			c.objectTypeRegistry = resource_objecttype_registry.NewObjectTypeRegistryResource()
			if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeRegistryResourceService(c.rootResourceMux, c.objectTypeRegistry); err != nil {
				return nil, err
			}

			// create and register the WorldOp registry on the root resource mux
			c.worldOpRegistry = resource_worldop_registry.NewWorldOpRegistryResource()
			if err := s4wave_worldop_registry.SRPCRegisterWorldOpRegistryResourceService(c.rootResourceMux, c.worldOpRegistry); err != nil {
				return nil, err
			}

			// create and register the ConfigType registry on the root resource mux
			c.configTypeRegistry = resource_configtype_registry.NewConfigTypeRegistryResource()
			if err := s4wave_configtype_registry.SRPCRegisterConfigTypeRegistryResourceService(c.rootResourceMux, c.configTypeRegistry); err != nil {
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

	bridgeCtrl := resource_objecttype_registry.NewBridgeController(le, b, c.objectTypeRegistry)
	bridgeRel, err := b.AddController(ctx, bridgeCtrl, nil)
	if err != nil {
		releaseAll()
		return err
	}
	releases = append(releases, bridgeRel)

	worldOpBridgeCtrl := resource_worldop_registry.NewWorldOpRegistryBridgeController(le, b, c.worldOpRegistry)
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
var _ controller.Controller = ((*Controller)(nil))
