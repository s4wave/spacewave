package sdk_world_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// SpaceWorldServiceID is the Resource service through which the Space plugin
// runtime serves a Space plugin the World engine of its Space, as the root
// Resource. A plugin reaches it on its host under bldr_plugin.HostServiceIDPrefix.
const SpaceWorldServiceID = "s4wave.world.SpaceWorld"

// SpaceWorldControllerID is the controller ID of SpaceWorldController.
const SpaceWorldControllerID = "s4wave/world/space-world"

// spaceWorldVersion is the controller version.
var spaceWorldVersion = controller.MustParseVersion("0.0.1")

// ConnectSpaceWorld opens the World engine that client serves under
// serviceID. The engine holds the connection until released.
func ConnectSpaceWorld(ctx context.Context, client srpc.Client, serviceID string) (*SDKEngine, error) {
	// Open the Resource client of the service.
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClientWithServiceID(client, serviceID))
	if err != nil {
		return nil, err
	}

	// Wrap its root, the engine, and close the client with it.
	engine, err := NewSDKEngine(resources, &closingRef{ResourceRef: resources.AccessRootResource(), client: resources})
	if err != nil {
		resources.Release()
		return nil, err
	}
	return engine, nil
}

// closingRef releases its Resource client with the reference.
type closingRef struct {
	resource_client.ResourceRef
	// client owns the connection of the reference.
	client *resource_client.Client
}

// Release releases the reference and its client.
func (r *closingRef) Release() {
	r.ResourceRef.Release()
	r.client.Release()
}

// SpaceWorldController resolves LookupWorldEngine in a Space plugin for the
// World of its Space, which its host serves under SpaceWorldServiceID, so a
// controller written for the daemon, such as a Flowgraph node's, runs in the
// plugin unchanged. Lookups of any other engine stay unresolved.
type SpaceWorldController struct {
	// b is the plugin bus.
	b bus.Bus
}

// NewSpaceWorldController constructs a SpaceWorldController on the plugin bus b.
func NewSpaceWorldController(b bus.Bus) *SpaceWorldController {
	return &SpaceWorldController{b: b}
}

// GetControllerInfo returns information about the controller.
func (c *SpaceWorldController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(SpaceWorldControllerID, spaceWorldVersion, "World of the plugin's Space")
}

// Execute does nothing: the controller resolves per lookup.
func (c *SpaceWorldController) Execute(context.Context) error {
	return nil
}

// HandleDirective resolves lookups of a named World engine.
func (c *SpaceWorldController) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(world.LookupWorldEngine)
	if !ok || d.LookupWorldEngineID() == "" {
		return nil, nil
	}
	engineID := d.LookupWorldEngineID()
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		return c.resolve(ctx, handler, engineID)
	}), nil)
}

// resolve publishes the Space's World while the host serves the World and the
// World's engine ID is engineID, marking the resolver idle once published.
// resolve returns when the host connection ends, so the lookup retries, or at
// once for another engine.
func (c *SpaceWorldController) resolve(ctx context.Context, handler directive.ResolverHandler, engineID string) error {
	// Reach the service on the plugin host, ending with the host client.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	serviceID := bldr_plugin.HostServiceIDPrefix + SpaceWorldServiceID
	clients, _, clientsRef, err := bifrost_rpc.ExLookupRpcClient(ctx, c.b, serviceID, SpaceWorldControllerID, true, cancel)
	if err != nil {
		return err
	}
	defer clientsRef.Release()

	// Open the World the host serves.
	engine, err := ConnectSpaceWorld(ctx, clients[0], serviceID)
	if err != nil {
		return err
	}
	defer engine.Release()

	// Publish the engine only if its ID is the one looked up.
	info, err := engine.GetEngineInfo(ctx)
	if err != nil {
		return err
	}
	if info.GetEngineInfo().GetEngineId() != engineID {
		return nil
	}
	id, _ := handler.AddValue(world.LookupWorldEngineValue(engine))
	defer handler.RemoveValue(id)
	handler.MarkIdle(true)
	<-ctx.Done()
	return context.Cause(ctx)
}

// Close releases the controller.
func (c *SpaceWorldController) Close() error {
	return nil
}

// _ is a type assertion.
var _ controller.Controller = (*SpaceWorldController)(nil)
