package space_exec

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// attachedWorldService writes through the engine supplied by the real bridge.
type attachedWorldService struct {
	entered chan struct{}
	exited  chan struct{}
}

func (s *attachedWorldService) Execute(ctx context.Context, req *PluginExecRequest) (*PluginExecResponse, error) {
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
	err = world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "plugin/result", nil)
		world.ReleaseObjectState(obj)
		return err
	})
	return &PluginExecResponse{Outputs: []*forge_value.Value{forge_value.NewValue("result")}}, err
}

func TestPluginExecAttachedWorldCancellationReachesPlugin(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	service := &attachedWorldService{entered: make(chan struct{}), exited: make(chan struct{})}
	root := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterPluginExecService(mux, service)
	})
	server := resource_server.NewResourceServer(root)
	mux := resource_server.NewResourceMux(server.Register)
	client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	handler := &pluginExecHandler{
		b: tb.Bus, le: tb.Logger, handle: &pluginExecHandleStub{},
		inputs: forge_target.InputMap{"world": forge_target.NewInputValueWorld(tb.Engine, tb.WorldState)},
	}
	done := make(chan error, 1)
	go func() { done <- handler.executeWithWorld(ctx, client, &PluginExecRequest{}) }()
	select {
	case <-service.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("plugin did not acquire its World")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled execution succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not join cancellation")
	}
	select {
	case <-service.exited:
	case <-time.After(2 * time.Second):
		t.Fatal("plugin retained its World after cancellation")
	}
}

func (s *attachedWorldService) ExecuteStream(req *PluginExecRequest, stream SRPCPluginExecService_ExecuteStreamStream) error {
	resp, err := s.Execute(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

func TestPluginExecAttachedWorldPublishesBeforeReturningOutput(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	root := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterPluginExecService(mux, &attachedWorldService{})
	})
	server := resource_server.NewResourceServer(root)
	mux := resource_server.NewResourceMux(server.Register)
	client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	handle := &pluginExecHandleStub{}
	handler := &pluginExecHandler{
		b:      tb.Bus,
		le:     tb.Logger,
		handle: handle,
		inputs: forge_target.InputMap{"world": forge_target.NewInputValueWorld(tb.Engine, tb.WorldState)},
	}
	if err := handler.executeWithWorld(ctx, client, &PluginExecRequest{}); err != nil {
		t.Fatal(err)
	}
	obj, found, err := tb.WorldState.GetObject(ctx, "plugin/result")
	world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("published object found=%v: %v", found, err)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "result" {
		t.Fatal("plugin output was not retained")
	}
}
