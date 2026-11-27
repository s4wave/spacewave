package space_exec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
)

// silentPluginInvoker accepts every plugin RPC and never answers, like a plugin
// whose bus holds the Resource lookup for a resolver that does not exist.
type silentPluginInvoker struct{}

// InvokeMethod blocks until the caller gives up.
func (silentPluginInvoker) InvokeMethod(_, _ string, strm srpc.Stream) (bool, error) {
	<-strm.Context().Done()
	return true, strm.Context().Err()
}

// TestPluginExecFailsWhenResourceServiceIsSilent runs the production Execution
// controller against a plugin that never answers the Resource client open. The
// Execution must fail within the bound and name the plugin and the cause.
func TestPluginExecFailsWhenResourceServiceIsSilent(t *testing.T) {
	// Serve a plugin whose Resource service accepts streams and stays silent.
	client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(silentPluginInvoker{}))))
	const bound = 300 * time.Millisecond
	registry := NewRegistry()
	registry.Register(PluginExecConfigID, newPluginExecHandlerWithOpenTimeout(nil, func(context.Context, bus.Bus, string, func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
		return client, nil, nil
	}, bound))
	tb, peerID := setupIntegrationTest(t, registry)

	// Create an Execution that opens the Resource client to attach the World.
	conf := &PluginExecConfig{PluginId: "silent-plugin", ControllerId: "example-controller", AttachWorld: true}
	configData, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	const execKey = "exec/silent-plugin"
	createTestExecution(t, t.Context(), tb.WorldState, peerID, execKey, PluginExecConfigID, configData)

	// The Execution completes as failed, within a few multiples of the bound.
	start := time.Now()
	ex := runTestExecution(t, tb, execKey, peerID)
	elapsed := time.Since(start)
	if ex.GetResult().IsSuccessful() {
		t.Fatal("execution with a silent plugin succeeded")
	}
	failError := ex.GetResult().GetFailError()
	for _, want := range []string{"silent-plugin", "did not answer its Resource service", bound.String()} {
		if !strings.Contains(failError, want) {
			t.Fatalf("fail error %q does not contain %q", failError, want)
		}
	}
	if elapsed > 10*bound {
		t.Fatalf("execution took %s to fail, bound is %s", elapsed, bound)
	}
	t.Logf("silent plugin failed the execution after %s: %s", elapsed, failError)
}
