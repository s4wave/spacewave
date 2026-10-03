package space_exec

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// TestPluginExecReceivesGrantedClaim forwards the native handle through both
// plugin RPC routes, including a claim granted after an earlier owner.
func TestPluginExecReceivesGrantedClaim(t *testing.T) {
	for _, attached := range []bool{false, true} {
		name := "ordinary"
		if attached {
			name = "attached-world"
		}
		t.Run(name, func(t *testing.T) {
			// Register a real plugin RPC service that captures decoded requests.
			service := &executionContextService{requests: make(chan *PluginExecRequest, 1)}
			root := resource_server.NewResourceMux(func(mux srpc.Mux) error {
				return SRPCRegisterPluginExecService(mux, service)
			})
			server := resource_server.NewResourceServer(root)
			mux := resource_server.NewResourceMux(server.Register, func(mux srpc.Mux) error {
				return SRPCRegisterPluginExecService(mux, service)
			})
			client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
			registry := NewRegistry()
			registry.Register(PluginExecConfigID, newPluginExecHandler(nil, func(context.Context, bus.Bus, string, func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
				return client, nil, nil
			}))
			tb, peerID := setupIntegrationTest(t, registry)

			// Create an Execution whose controller config must survive the bridge.
			conf := &PluginExecConfig{
				PluginId: "example-plugin", ControllerId: "example-controller",
				ControllerConfig: []byte{4, 5, 6}, AttachWorld: attached,
			}
			configData, err := conf.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			const execKey = "exec/plugin-granted-claim"
			createTestExecution(t, t.Context(), tb.WorldState, peerID, execKey, PluginExecConfigID, configData)
			controllerConf := execution_controller.NewConfig(tb.EngineID, execKey, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})

			// Grant the native controller epoch two through the authoritative claim transactions.
			obj, err := world.MustGetObject(t.Context(), tb.WorldState, execKey)
			defer world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := obj.ApplyObjectOp(t.Context(), execution_tx.NewTxStart(peerID, time.Now().Add(-time.Minute), "previous-owner"), peerID); err != nil {
				t.Fatal(err)
			}
			if _, _, err := obj.ApplyObjectOp(t.Context(), execution_tx.NewTxReclaim(peerID, controllerConf.GetClaimId(), 1, time.Now(), time.Now().Add(time.Hour)), peerID); err != nil {
				t.Fatal(err)
			}

			// Run the production controller and inspect the request decoded by the plugin.
			assertComplete(t, runTestExecution(t, tb, execKey, peerID))
			req := <-service.requests
			if req.GetExecutionObjectKey() != execKey || req.GetClaimEpoch() != 2 {
				t.Fatalf("granted execution = %q epoch %d, want %q epoch 2", req.GetExecutionObjectKey(), req.GetClaimEpoch(), execKey)
			}
			if (req.GetAttachedEngineResourceId() != 0) != attached {
				t.Fatalf("attached World resource = %d, attached=%v", req.GetAttachedEngineResourceId(), attached)
			}
			if req.GetControllerId() != conf.GetControllerId() || !bytes.Equal(req.GetControllerConfig(), conf.GetControllerConfig()) {
				t.Fatalf("controller config changed in transit: %+v", req)
			}
		})
	}
}
