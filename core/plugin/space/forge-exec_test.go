package plugin_space

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
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

// manifestPluginHost loads one plugin once its manifest resolves, as the
// plugin host does: the load stays busy while FetchManifest is busy and goes
// idle without a plugin when FetchManifest has no manifest.
type manifestPluginHost struct {
	// b is the bus FetchManifest resolves on.
	b bus.Bus
	// client is the running plugin's RPC client.
	client srpc.Client
}

// GetControllerInfo returns information about the controller.
func (h *manifestPluginHost) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/manifest-plugin-host",
		controller.MustParseVersion("0.0.1"),
		"loads a test plugin once its manifest resolves",
	)
}

// Execute waits for the controller context to end.
func (h *manifestPluginHost) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close releases any resources used by the controller.
func (h *manifestPluginHost) Close() error { return nil }

// HandleDirective resolves LoadPlugin for the test plugin from its manifest.
func (h *manifestPluginHost) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || load.LoadPluginID() != spacePluginID {
		return nil, nil
	}

	// Publish the running plugin once its manifest becomes available.
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		// Follow the plugin manifest until the load ends.
		fetch, fetchRef, err := h.b.AddDirective(
			bldr_manifest.NewFetchManifest(spacePluginID, nil, []string{spacePlatformID}, 0),
			nil,
		)
		if err != nil {
			return err
		}
		defer fetchRef.Release()
		loaded := false
		releaseState := fetch.AddStateCallback(func(isIdle bool, _ []error, vals []directive.AttachedValue) {
			for _, val := range vals {
				fetched, ok := val.GetValue().(*bldr_manifest.FetchManifestValue)
				if ok && len(fetched.GetManifestRefs()) != 0 && !loaded {
					loaded = true
					_, _ = handler.AddValue(bldr_plugin.NewRunningPlugin(h.client))
				}
			}
			handler.MarkIdle(isIdle || loaded)
		})
		defer releaseState()
		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

// _ verifies the controller contract.
var _ controller.Controller = (*manifestPluginHost)(nil)

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
	space_exec.RegisterPluginExec(registry, tb.Bus)
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
	service := &execRecorder{requests: make(chan *space_exec.PluginExecRequest, 1)}
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
		return e.GetWaitingPluginId() != ""
	})
	if got := waiting.GetWaitingPluginId(); got != spacePluginID {
		t.Fatalf("waiting plugin = %q, want %q", got, spacePluginID)
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

	// Listing the plugin lets the same Execution run to completion.
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
	completed := waitExecution(ctx, t, tb, execKey, (*forge_execution.Execution).IsComplete)
	if !completed.GetResult().IsSuccessful() {
		t.Fatalf("Execution failed: %s", completed.GetResult().GetFailError())
	}
	if got := completed.GetWaitingPluginId(); got != "" {
		t.Fatalf("completed waiting plugin = %q", got)
	}
	if req := <-service.requests; req.GetExecutionObjectKey() != execKey {
		t.Fatalf("plugin ran Execution %q, want %q", req.GetExecutionObjectKey(), execKey)
	}
}

// execRecorder is a plugin exec service that records the Executions it runs.
type execRecorder struct {
	// requests delivers each request the plugin receives.
	requests chan *space_exec.PluginExecRequest
}

// Execute records the request and completes without output.
func (r *execRecorder) Execute(_ context.Context, req *space_exec.PluginExecRequest) (*space_exec.PluginExecResponse, error) {
	r.requests <- req.CloneVT()
	return &space_exec.PluginExecResponse{}, nil
}

// ExecuteStream sends the unary result over the streaming interface.
func (r *execRecorder) ExecuteStream(req *space_exec.PluginExecRequest, stream space_exec.SRPCPluginExecService_ExecuteStreamStream) error {
	resp, err := r.Execute(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

// _ verifies the plugin service contract.
var _ space_exec.SRPCPluginExecServiceServer = (*execRecorder)(nil)

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
