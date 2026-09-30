package space_exec

import (
	"context"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// scopedWorldService observes typed invocation through the real borrowed World mount.
type scopedWorldService struct {
	// engineID is the ID selected by the execution's World input.
	engineID string
	// objectKey names the object with an engine-scoped registration.
	objectKey string
}

// Execute invokes the scoped handler through both the engine and its transaction.
func (s *scopedWorldService) Execute(ctx context.Context, req *PluginExecRequest) (*PluginExecResponse, error) {
	// Inspect the identity on the engine actually lent by the production bridge.
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineRPC, err := owner.GetAttachedResource(req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	engine := sdk_world.NewSRPCEngineResourceServiceClient(engineRPC)
	info, err := engine.GetEngineInfo(ctx, &sdk_world.GetEngineInfoRequest{})
	if err != nil {
		return nil, err
	}
	if info.GetEngineInfo().GetEngineId() != s.engineID {
		return nil, errors.New("borrowed mount dropped the selected identity")
	}

	// Typed access on the engine must select the scoped registration.
	client := sdk_world_engine.NewAttachedResourceClient(owner)
	if err := s.invokeTyped(ctx, client, sdk_world.NewSRPCTypedObjectResourceServiceClient(engineRPC)); err != nil {
		return nil, err
	}

	// Transaction descendants must retain that same trusted mount identity.
	tx, err := engine.NewTransaction(ctx, &sdk_world.NewTransactionRequest{})
	if err != nil {
		return nil, err
	}
	txRef := client.CreateResourceReference(tx.GetResourceId())
	defer txRef.Release()
	txRPC, err := txRef.GetClient()
	if err != nil {
		return nil, err
	}
	if err := s.invokeTyped(ctx, client, sdk_world.NewSRPCTypedObjectResourceServiceClient(txRPC)); err != nil {
		return nil, err
	}
	return &PluginExecResponse{}, nil
}

// invokeTyped acquires, invokes and releases one typed child on the borrowed tree.
func (s *scopedWorldService) invokeTyped(ctx context.Context, client *sdk_world_engine.AttachedResourceClient, typed sdk_world.SRPCTypedObjectResourceServiceClient) error {
	// Acquire the child through the production typed-object RPC.
	access, err := typed.AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{ObjectKey: s.objectKey})
	if err != nil {
		return err
	}
	ref := client.CreateResourceReference(access.GetResourceId())
	defer ref.Release()
	rpc, err := ref.GetClient()
	if err != nil {
		return err
	}

	// Invoke the registry handler's child through the real Resource route.
	resp, err := echo.NewSRPCEchoerClient(rpc).Echo(ctx, &echo.EchoMsg{Body: "scoped-handler"})
	if err != nil {
		return err
	}
	if resp.GetBody() != "scoped-handler" {
		return errors.New("typed handler returned the wrong response")
	}
	return nil
}

// ExecuteStream preserves the execution service's explicit streaming entry point.
func (s *scopedWorldService) ExecuteStream(req *PluginExecRequest, stream SRPCPluginExecService_ExecuteStreamStream) error {
	resp, err := s.Execute(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

// _ is a type assertion.
var _ SRPCPluginExecServiceServer = (*scopedWorldService)(nil)
