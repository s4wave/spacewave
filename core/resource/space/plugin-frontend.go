package resource_space

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/world"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	uuid "github.com/satori/go.uuid"
)

// OpenPluginFrontend grants one Resource a live compiler on the selected device.
// The source stays in this World; the Resource owns the temporary execution grant.
func (r *SpaceResource) OpenPluginFrontend(ctx context.Context, request *s4wave_space.BuildSpacePluginRequest) (*s4wave_space.OpenPluginFrontendResponse, error) {
	resources, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	sender, err := confparse.ParsePeerID(r.sessionPeerID)
	if err != nil {
		return nil, err
	}
	shared := r.space.GetSharedObject()
	if shared == nil {
		return nil, errors.New("frontend authoring requires the mounted Session transport")
	}

	lifetime, cancel := context.WithCancel(resources.Context())
	transportBus, releaseTransport, err := transport.ResolveSessionBus(ctx, r.b, sender, cancel)
	if err != nil {
		cancel()
		return nil, err
	}

	// The normal queue path checks the source and device's active worker grant.
	id := uuid.NewV4().String()
	servicePrefix := frontend.AttachedServicePrefix + id + "/"
	serviceID := servicePrefix + frontend.SRPCFrontendServiceID
	if pluginID := r.resolveHostPluginID(ctx); pluginID != "" {
		serviceID = bldr_plugin.PluginServiceID(pluginID, serviceID)
	}
	queued, err := r.queuePluginBuild(ctx, request, &space_exec.PluginBuildConfig{
		FrontendId: id, FrontendRoutePrefix: frontend.ServiceRoutePrefix(serviceID),
	})
	if err != nil {
		releaseTransport()
		cancel()
		return nil, err
	}
	client := srpc.NewClient(stream_srpc.NewOpenStreamFunc(transportBus, space_exec.PluginFrontendProtocol(id), sender, queued.peer, 0))
	var forward srpc.Invoker = srpc.NewClientInvoker(client)
	if sender == queued.peer {
		// A device running this Session serves its compiler on the local bus;
		// authenticated network transports deliberately reject self-dialing.
		local := bifrost_rpc.NewInvoker(r.b, "space/plugin-frontend", true)
		prefix := string(space_exec.PluginFrontendProtocol(id)) + "/"
		forward = srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
			return local.InvokeMethod(prefix+serviceID, methodID, stream)
		})
	}

	// Every forwarded call ends with this attachment, including a pending dial.
	mux := srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		if serviceID != frontend.SRPCFrontendServiceID {
			return false, nil
		}
		callCtx, stop := context.WithCancel(stream.Context())
		stopCancel := context.AfterFunc(lifetime, func() {
			stop()
			// Close interrupts reads in the borrowed stream as well as forwarding.
			_ = stream.Close()
		})
		defer stopCancel()
		defer stop()
		return forward.InvokeMethod(serviceID, methodID, srpc.NewStreamWithContext(stream, callCtx))
	})
	var releaseRoute func()
	closeAttachment := func() {
		cancel()
		releaseTransport()
		if releaseRoute != nil {
			releaseRoute()
		}

		// Cancellation is an accepted Forge operation, including before a device
		// starts. The Resource release joins its bounded write-back attempt.
		closeCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		ws := world.NewEngineWorldState(r.space.GetWorldEngine(), true)
		object, closeErr := world.MustGetObject(closeCtx, ws, queued.key)
		defer world.ReleaseObjectState(object)
		if closeErr == nil {
			_, _, closeErr = object.ApplyObjectOp(closeCtx, execution_tx.NewTxCancel(), sender)
		}
		if closeErr != nil {
			r.le.WithError(closeErr).Warn("cancel plugin frontend attachment")
		}
	}

	// Browser module fetches use the same bounded connection as the Resource.
	route := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("space/plugin-frontend-route", controller.MustParseVersion("0.0.1"), "Space plugin frontend route"),
		bifrost_rpc.NewRpcServiceBuilder(mux), []string{servicePrefix}, true, nil, nil, nil)
	releaseRoute, err = r.b.AddController(lifetime, route, nil)
	if err != nil {
		closeAttachment()
		return nil, err
	}
	resourceID, err := resources.AddResource(mux, closeAttachment)
	if err != nil {
		closeAttachment()
		return nil, err
	}
	return &s4wave_space.OpenPluginFrontendResponse{ResourceId: resourceID, ExecutionKey: queued.key}, nil
}
