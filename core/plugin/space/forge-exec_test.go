package plugin_space

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_target "github.com/s4wave/spacewave/forge/target"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	bifrost_rpc_access "github.com/s4wave/spacewave/net/rpc/access"
	"github.com/s4wave/spacewave/testbed"
)

const (
	// spacePluginID identifies the manifest required by the test Execution.
	spacePluginID = "unlisted-plugin"
	// spacePlatformID selects the test manifest platform.
	spacePlatformID = "test/platform"
)

// newPluginExecClient serves service as a plugin's RPC client, reached through
// the access service as the plugin host exposes it.
func newPluginExecClient(t *testing.T, tb *testbed.Testbed, service space_exec.SRPCPluginExecServiceServer) srpc.Client {
	// Expose the plugin exec service on the plugin's own bus.
	t.Helper()
	pluginBus, _, err := controllerbus_core.NewCoreBus(t.Context(), tb.Logger)
	if err != nil {
		t.Fatal(err)
	}

	// Register the service the plugin bus exposes to remote callers.
	execMux := srpc.NewMux()
	if err := space_exec.SRPCRegisterPluginExecService(execMux, service); err != nil {
		t.Fatal(err)
	}
	invoker := bifrost_rpc.NewInvokerController(
		tb.Logger,
		pluginBus,
		controller.NewInfo("test/plugin-exec-invoker", controller.MustParseVersion("0.0.1"), ""),
		execMux,
		nil,
	)
	releaseInvoker, err := pluginBus.AddController(t.Context(), invoker, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseInvoker)

	// Serve the access service that routes lookups to that bus.
	accessMux := srpc.NewMux()
	if err := bifrost_rpc_access.SRPCRegisterAccessRpcService(
		accessMux,
		bifrost_rpc_access.NewAccessRpcServiceServer(pluginBus, true, nil),
	); err != nil {
		t.Fatal(err)
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(accessMux)))
}

// waitExecution returns the first revision of the Execution that satisfies done.
func waitExecution(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	execKey string,
	done func(*forge_execution.Execution) bool,
) *forge_execution.Execution {
	// Watch the Execution object until a revision satisfies done.
	t.Helper()
	var found *forge_execution.Execution
	loop := world_control.NewWatchLoop(tb.Logger, execKey, world_control.NewWaitForStateHandler(
		func(ctx context.Context, _ world.WorldState, obj world.ObjectState, rootCs *block.Cursor, _ uint64) (bool, error) {
			// Keep watching until the Execution has a readable state.
			if obj == nil {
				return true, nil
			}

			// Stop at the first Execution revision accepted by the caller.
			exec, err := forge_execution.UnmarshalExecution(ctx, rootCs)
			if err != nil {
				return false, err
			}
			if done(exec) {
				found = exec
				return false, nil
			}
			return true, nil
		},
	))
	if err := loop.Execute(ctx, tb.WorldState); err != nil {
		t.Fatal(err)
	}
	return found
}

// TestPluginExecWaitsForUnlistedSpacePlugin runs an Execution through the Space
// plugin controller for a plugin the Space does not list. The Execution reports the
// plugin it waits for, and listing the plugin lets the same Execution complete.
func TestPluginExecWaitsForUnlistedSpacePlugin(t *testing.T) {
	// Bound the Execution and plugin demand waits.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Start the Space and register its Execution controller factories.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	peerID := tb.Volume.GetPeerID()
	registry := space_exec.NewRegistry()
	registry.Register(space_exec.PluginExecConfigID, space_exec.NewPluginExecHandler(tb.Bus))
	tb.StaticResolver.AddFactory(execution_controller.NewFactory(tb.Bus))
	for _, factory := range space_exec.BridgeFactories(registry) {
		tb.StaticResolver.AddFactory(factory)
	}

	// Store the plugin manifest in the Space, without listing the plugin.
	const storeKey = "test/plugin-manifests"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, storeKey); err != nil {
		t.Fatal(err)
	}
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx, tb.WorldState, peerID, "test/manifests/plugin", []string{storeKey}, createSpacePluginManifest(ctx, t, tb),
	); err != nil {
		t.Fatal(err)
	}

	// Run the Space plugin controller and a host that loads from its manifests.
	service := &execRecorder{
		requests: make(chan *space_exec.PluginExecRequest, 1),
		finish:   make(chan struct{}),
	}
	host := &manifestPluginHost{b: tb.Bus, client: newPluginExecClient(t, tb, service)}
	releaseHost, err := tb.Bus.AddController(ctx, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHost()

	// Resolve the Space plugin controller against the same manifest store.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	spaceCtrl, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: peerID.String(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer spaceRef.Release()

	// Start an Execution that needs the unlisted plugin.
	configData, err := (&space_exec.PluginExecConfig{PluginId: spacePluginID, ControllerId: "example-controller"}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	const execKey = "exec/unlisted-plugin"
	target := &forge_target.Target{Exec: &forge_target.Exec{Controller: &configset_proto.ControllerConfig{
		Id: space_exec.PluginExecConfigID, Rev: 1, Config: configData,
	}}}
	if _, err := forge_execution.CreateExecutionWithTarget(
		ctx, tb.WorldState, peerID, execKey, peerID, nil, target, nil, timestamp.Now(),
	); err != nil {
		t.Fatal(err)
	}

	// Run the Execution against the Space World.
	controllerConf := execution_controller.NewConfig(tb.EngineID, execKey, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
	_, ctrlRef, err := execution_controller.StartControllerWithConfig(ctx, tb.Bus, controllerConf)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()

	// Wait for the Execution to report the plugin it needs.
	waiting := waitExecution(ctx, t, tb, execKey, func(e *forge_execution.Execution) bool {
		return e.GetWaiting().GetPluginId() != ""
	})
	if got := waiting.GetWaiting().GetPluginId(); got != spacePluginID {
		t.Fatalf("waiting plugin = %q, want %q", got, spacePluginID)
	}
	if state := waiting.GetExecutionState(); state != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("waiting Execution state = %s, want RUNNING", state)
	}

	// Keep plugin side effects blocked until the Space lists the plugin.
	select {
	case req := <-service.requests:
		t.Fatalf("unlisted plugin ran Execution %q", req.GetExecutionObjectKey())
	default:
	}

	// The manifest resolver publishes its demand after the Execution status.
	for {
		requested, waitCh := spaceCtrl.GetRequestedPluginIDsAndWaitCh()
		if slices.Contains(requested, spacePluginID) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("requested plugins = %v, want %q: %v", requested, spacePluginID, ctx.Err())
		case <-waitCh:
		}
	}

	// Listing the plugin lets the same Execution enter the plugin service.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		peerID,
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{spacePluginID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case req := <-service.requests:
		if req.GetExecutionObjectKey() != execKey {
			t.Fatalf("plugin ran Execution %q, want %q", req.GetExecutionObjectKey(), execKey)
		}
		if got := req.GetClaimEpoch(); got != waiting.GetClaim().GetEpoch() {
			t.Fatalf("plugin claim epoch = %d, want %d", got, waiting.GetClaim().GetEpoch())
		}
	}

	// The plugin load clears the wait before the Execution completes.
	proceeding := waitExecution(ctx, t, tb, execKey, func(e *forge_execution.Execution) bool {
		return e.GetWaiting().GetPluginId() == ""
	})
	if state := proceeding.GetExecutionState(); state != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("proceeding Execution state = %s, want RUNNING", state)
	}

	// Finish the same Execution and verify the terminal state keeps the wait clear.
	close(service.finish)
	completed := waitExecution(ctx, t, tb, execKey, (*forge_execution.Execution).IsComplete)
	if !completed.GetResult().IsSuccessful() {
		t.Fatalf("Execution failed: %s", completed.GetResult().GetFailError())
	}
	if got := completed.GetWaiting().GetPluginId(); got != "" {
		t.Fatalf("completed waiting plugin = %q", got)
	}

	// The same Execution finishes under the claim it held while waiting.
	if got := completed.GetClaim(); got.GetClaimId() != waiting.GetClaim().GetClaimId() || got.GetEpoch() != waiting.GetClaim().GetEpoch() {
		t.Fatalf("Execution claim changed from %v to %v", waiting.GetClaim(), got)
	}
}

// createSpacePluginManifest writes the test plugin's manifest into the Space
// bucket and returns its reference.
func createSpacePluginManifest(ctx context.Context, t *testing.T, tb *testbed.Testbed) *bldr_manifest.ManifestRef {
	// Describe the manifest and stage it until the test ends.
	t.Helper()
	meta := bldr_manifest.NewManifestMeta(spacePluginID, bldr_manifest.BuildType_DEV, spacePlatformID, 1)
	stage, err := tb.Engine.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stage.Release)

	// Write the manifest block and point the reference at it.
	var manifestRef *bldr_manifest.ManifestRef
	if err := stage.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Commit the manifest block through the staging transaction.
		transaction, blocks := cursor.BuildTransactionAtRef(nil, nil)
		blocks.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		rootRef, _, err := transaction.Write(ctx, true)
		if err != nil {
			return err
		}

		// Return a reference to the committed manifest.
		objectRef := cursor.GetRef().CloneVT()
		objectRef.RootRef = rootRef
		manifestRef = bldr_manifest.NewManifestRef(meta, objectRef)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return manifestRef
}
