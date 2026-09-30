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
)

// TestPluginExecAttachedWorldCancellationReachesPlugin observes cancellation through the real borrowed Resource tree.
func TestPluginExecAttachedWorldCancellationReachesPlugin(t *testing.T) {
	// Serve a plugin that waits after acquiring its attached World.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	service := &attachedWorldService{entered: make(chan struct{}), exited: make(chan struct{})}
	root := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterPluginExecService(mux, service)
	})
	server := resource_server.NewResourceServer(root)
	mux := resource_server.NewResourceMux(server.Register)

	// Connect the plugin and grant its selected transactional World.
	client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	handler := &pluginExecHandler{
		b: tb.Bus, le: tb.Logger, handle: &pluginExecHandleStub{},
		inputs: forge_target.InputMap{"world": forge_target.NewInputValueWorld(tb.EngineID, tb.Engine, tb.WorldState)},
	}

	// Wait for plugin acquisition before canceling the borrowed execution.
	done := make(chan error, 1)
	go func() { done <- handler.executeWithWorld(ctx, client, &PluginExecRequest{}) }()
	select {
	case <-service.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("plugin did not acquire its World")
	}

	// Cancel execution and join the bridge and plugin event barriers.
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

// TestPluginExecAttachedWorldPublishesBeforeReturningOutput checks the committed World result before exposing outputs.
func TestPluginExecAttachedWorldPublishesBeforeReturningOutput(t *testing.T) {
	// Serve the plugin and lend its selected transactional World.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	root := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterPluginExecService(mux, &attachedWorldService{})
	})
	server := resource_server.NewResourceServer(root)
	mux := resource_server.NewResourceMux(server.Register)
	client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Execute through the production borrowed World bridge.
	handle := &pluginExecHandleStub{}
	handler := &pluginExecHandler{
		b:      tb.Bus,
		le:     tb.Logger,
		handle: handle,
		inputs: forge_target.InputMap{"world": forge_target.NewInputValueWorld(tb.EngineID, tb.Engine, tb.WorldState)},
	}
	if err := handler.executeWithWorld(ctx, client, &PluginExecRequest{}); err != nil {
		t.Fatal(err)
	}

	// Read the committed object and execution output after the bridge returns.
	obj, found, err := tb.WorldState.GetObject(ctx, "plugin/result")
	world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("published object found=%v: %v", found, err)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "result" {
		t.Fatal("plugin output was not retained")
	}
}
