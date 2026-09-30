//go:build !js

package resource_world_test

import (
	"context"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_types "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// typedWatchHandler returns distinguishable children through a real attached ResourceService.
type typedWatchHandler struct {
	// label distinguishes invocations of this handler generation.
	label string
	// engineID is the expected trusted scope on the borrowed engine.
	engineID string
	// server owns the handler's nested Resource clients and children.
	server *resource_server.ResourceServer
	// started reports factory entry before any optional gate.
	started chan struct{}
	// gate blocks construction until released or the request cancels.
	gate <-chan struct{}
	// canceled reports factory cancellation to the test.
	canceled chan struct{}
	// released reports child cleanup independently of the parent connection.
	released chan struct{}
	// calls records actual child methods, exposing accidental replay.
	calls chan struct{}
}

// newTypedWatchHandler constructs callback barriers for one admitted generation.
func newTypedWatchHandler(label, engineID string) *typedWatchHandler {
	return &typedWatchHandler{label: label, engineID: engineID, started: make(chan struct{}, 64), canceled: make(chan struct{}, 64), released: make(chan struct{}, 64), calls: make(chan struct{}, 64)}
}

// InvokeObjectType checks trusted scope and constructs the registered child resource.
func (h *typedWatchHandler) InvokeObjectType(ctx context.Context, req *sdk_types.InvokeObjectTypeRequest) (*sdk_types.InvokeObjectTypeResponse, error) {
	// Publish factory entry and honor request cancellation while gated.
	h.started <- struct{}{}
	if h.gate != nil {
		select {
		case <-ctx.Done():
			h.canceled <- struct{}{}
			return nil, ctx.Err()
		case <-h.gate:
		}
	}

	// Inspect the borrowed engine's actual identity over Resource RPC.
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineClient, err := owner.GetAttachedResource(req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	info, err := sdk_world.NewSRPCEngineResourceServiceClient(engineClient).GetEngineInfo(ctx, &sdk_world.GetEngineInfoRequest{})
	if err != nil {
		return nil, err
	}
	if info.GetEngineInfo().GetEngineId() != h.engineID {
		return nil, errors.Errorf("borrowed engine scope = %q, want %q", info.GetEngineInfo().GetEngineId(), h.engineID)
	}

	// Return a real invocable child with an independently observed release callback.
	mux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		// Match the Echo child method before reading its request.
		if serviceID != echo.SRPCEchoerServiceID || methodID != "Echo" {
			return false, nil
		}
		if err := stream.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}

		// Record the caller's explicit invocation and identify this generation.
		h.calls <- struct{}{}
		return true, stream.MsgSend(&echo.EchoMsg{Body: h.label})
	}))
	id, err := owner.AddResource(mux, func() { h.released <- struct{}{} })
	if err != nil {
		return nil, err
	}
	return &sdk_types.InvokeObjectTypeResponse{ResourceId: id}, nil
}

// _ is a type assertion.
var _ sdk_types.SRPCObjectTypeHandlerServiceServer = (*typedWatchHandler)(nil)
