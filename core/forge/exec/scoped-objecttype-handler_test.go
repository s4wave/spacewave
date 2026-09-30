package space_exec

import (
	"context"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// scopedObjectTypeHandler serves a real child through a caller-attached registry handler.
type scopedObjectTypeHandler struct {
	// engineID is the expected trusted scope of the engine lent to the handler.
	engineID string
	// released reports release of the handler's child through Resource control.
	released chan struct{}
}

// InvokeObjectType validates the lent engine before exposing an invocable child.
func (h *scopedObjectTypeHandler) InvokeObjectType(ctx context.Context, req *sdk_registry.InvokeObjectTypeRequest) (*sdk_registry.InvokeObjectTypeResponse, error) {
	// Observe the engine scope through the actual attached Resource service.
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
		return nil, errors.New("handler received the wrong engine")
	}

	// Retain the child until its invocation Resource is released.
	mux := srpc.NewMux()
	if err := echo.NewEchoServer(mux).Register(mux); err != nil {
		return nil, err
	}
	id, err := owner.AddResource(mux, func() { h.released <- struct{}{} })
	if err != nil {
		return nil, err
	}
	return &sdk_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

// _ is a type assertion.
var _ sdk_registry.SRPCObjectTypeHandlerServiceServer = (*scopedObjectTypeHandler)(nil)
