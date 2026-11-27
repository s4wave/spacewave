package plugin_space_runtime

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// worldServiceControllerID is the controller ID of the Space World service.
const worldServiceControllerID = "plugin/space/runtime/world"

// worldService serves the Space's World engine to the generation's plugins as
// the root of a Resource client, under sdk_world_engine.SpaceWorldServiceID.
// Plugins reach it through their host, so a plugin controller, such as a
// Flowgraph node's, can use the World of its own Space and no other.
type worldService struct {
	// le is the logger.
	le *logrus.Entry
	// b is the generation bus, which bridges the engine and its operations
	// from the parent.
	b bus.Bus
	// engineID is the World engine of the Space.
	engineID string
	// sessionPeerID is the session peer that writes as the Space's mount.
	sessionPeerID peer.ID
}

// newWorldService constructs the World service of the Space whose World engine
// is engineID, writing as sessionPeerID.
func newWorldService(le *logrus.Entry, b bus.Bus, engineID string, sessionPeerID peer.ID) *worldService {
	return &worldService{le: le, b: b, engineID: engineID, sessionPeerID: sessionPeerID}
}

// GetControllerInfo returns information about the controller.
func (s *worldService) GetControllerInfo() *controller.Info {
	return controller.NewInfo(worldServiceControllerID, Version, "Space World for its plugins")
}

// Execute does nothing: the service resolves per lookup.
func (s *worldService) Execute(context.Context) error {
	return nil
}

// HandleDirective serves the World to a plugin's lookup of the service. The
// plugin host records the looking-up plugin in the lookup's server ID. A lookup
// that specifies no plugin gets no World.
func (s *worldService) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Match a lookup of the Space World service.
	d, ok := di.GetDirective().(bifrost_rpc.LookupRpcService)
	if !ok || d.LookupRpcServiceID() != sdk_world_engine.SpaceWorldServiceID {
		return nil, nil
	}

	// Serve the World to the plugin the lookup specifies.
	pluginID, ok := strings.CutPrefix(d.LookupRpcServerID(), bldr_plugin.PluginServerIDPrefix)
	if !ok || pluginID == "" {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		return s.resolve(ctx, handler, pluginID)
	}), nil)
}

// resolve serves the World to the plugin pluginID until the lookup ends. Each
// lookup holds the engine and a Resource server whose root is the engine's
// Resource.
func (s *worldService) resolve(ctx context.Context, handler directive.ResolverHandler, pluginID string) error {
	// Hold the Space's World engine, which the bridge resolves on the parent.
	engine, _, engineRef, err := world.ExLookupWorldEngine(ctx, s.b, false, s.engineID, nil)
	if err != nil {
		return err
	}
	defer engineRef.Release()

	// Serve the engine as the root Resource, writing as the Space's mount
	// within what the plugin may write.
	root := resource_world.NewEngineResource(
		s.le,
		s.b,
		engine,
		space_world_optypes.BuildSpaceLookupOp(s.b, s.le, s.engineID),
		&s4wave_world.EngineInfo{EngineId: s.engineID},
		resource_world.WithSessionPeerID(s.sessionPeerID),
		resource_world.WithWriteGuard(space_world_optypes.NewPluginWriteGuard(pluginID)),
	)
	defer root.Close()
	server := resource_server.NewResourceServer(root.GetMux())
	server.SetOriginController(worldServiceControllerID)
	mux := srpc.NewMux()
	if err := mux.Register(resource.NewSRPCResourceServiceHandler(server, sdk_world_engine.SpaceWorldServiceID)); err != nil {
		return err
	}

	// Publish the service until the lookup ends. Marking idle lets the
	// plugin host's lookup, which waits for idle, take the value.
	handler.AddValue(bifrost_rpc.LookupRpcServiceValue(mux))
	handler.MarkIdle(true)
	<-ctx.Done()
	return nil
}

// Close releases the controller.
func (s *worldService) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*worldService)(nil)
