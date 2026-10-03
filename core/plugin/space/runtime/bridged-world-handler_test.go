package plugin_space_runtime

import (
	"context"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// bridgedWorldHandler validates the borrowed World before exposing an Echo child.
type bridgedWorldHandler struct {
	// engineID is the trusted scope expected from the child World mount.
	engineID string
}

// InvokeObjectType serves a child only for the typed object in its granted engine.
func (h *bridgedWorldHandler) InvokeObjectType(ctx context.Context, req *sdk_registry.InvokeObjectTypeRequest) (*sdk_registry.InvokeObjectTypeResponse, error) {
	// Check the object selected by the attached World's typed watch.
	if req.GetTypeId() != "test/bridged-type" || req.GetObjectKey() != "test/bridged-object" {
		return nil, errors.New("handler received the wrong typed object")
	}

	// Inspect the engine identity through the actual borrowed Resource capability.
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineRPC, err := owner.GetAttachedResource(req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	info, err := sdk_world.NewSRPCEngineResourceServiceClient(engineRPC).GetEngineInfo(ctx, &sdk_world.GetEngineInfoRequest{})
	if err != nil {
		return nil, err
	}
	if info.GetEngineInfo().GetEngineId() != h.engineID {
		return nil, errors.New("handler received the wrong engine scope")
	}

	// Expose an independently owned child for explicit invocation by the consumer.
	mux := srpc.NewMux()
	if err := echo.NewEchoServer(mux).Register(mux); err != nil {
		return nil, err
	}
	id, err := owner.AddResource(mux, nil)
	if err != nil {
		return nil, err
	}
	return &sdk_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

// _ is a type assertion.
var _ sdk_registry.SRPCObjectTypeHandlerServiceServer = (*bridgedWorldHandler)(nil)
