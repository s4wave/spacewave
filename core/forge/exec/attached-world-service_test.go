package space_exec

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	forge_value "github.com/s4wave/spacewave/forge/value"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// attachedWorldService writes through the engine supplied by the real bridge.
type attachedWorldService struct {
	// entered reports acquisition of the borrowed World.
	entered chan struct{}
	// exited reports cancellation of the borrowed execution.
	exited chan struct{}
}

// Execute writes through the borrowed World or waits for execution cancellation.
func (s *attachedWorldService) Execute(ctx context.Context, req *PluginExecRequest) (*PluginExecResponse, error) {
	// Acquire the attached engine until this execution returns.
	engine, err := sdk_world_engine.NewAttachedEngine(ctx, req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	defer engine.Release()
	if s.entered != nil {
		defer close(s.exited)
		close(s.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	// Publish the plugin result before returning its output.
	err = world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "plugin/result", nil)
		world.ReleaseObjectState(obj)
		return err
	})
	return &PluginExecResponse{Outputs: []*forge_value.Value{forge_value.NewValue("result")}}, err
}

// ExecuteStream forwards the execution outcome on its explicit streaming route.
func (s *attachedWorldService) ExecuteStream(req *PluginExecRequest, stream SRPCPluginExecService_ExecuteStreamStream) error {
	resp, err := s.Execute(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

// _ is a type assertion.
var _ SRPCPluginExecServiceServer = (*attachedWorldService)(nil)
