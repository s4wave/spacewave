package resource_space

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	plugin_host_resource "github.com/s4wave/spacewave/bldr/plugin/host/resource"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	plugin_space_runtime "github.com/s4wave/spacewave/core/plugin/space/runtime"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// bindingTestPolicy supplies a stable daemon policy to the real Worker.
type bindingTestPolicy struct {
	// data is the encoded Device policy revision.
	data []byte
}

// WaitDevicePolicy returns the current revision, then awaits cancellation.
func (p *bindingTestPolicy) WaitDevicePolicy(ctx context.Context, last []byte) ([]byte, string, uint64, error) {
	if len(last) == 0 {
		return p.data, "devices/self", 1, nil
	}
	<-ctx.Done()
	return nil, "", 0, ctx.Err()
}

// addBindingTestWorkerPolicyHost serves the policy Resource route through the
// Space scheduler's plugin-host client.
func addBindingTestWorkerPolicyHost(t *testing.T, ctx context.Context, tb *testbed.Testbed, runtime *plugin_space_runtime.Controller, workerKey string) {
	t.Helper()
	policy, err := (&device_policy.DevicePolicy{Revision: 1, ForgeWorker: &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: workerKey, MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{"docker"},
	}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	hostRoot := plugin_host_root.NewRoot()
	hostRoot.SetDevicePolicySource(&bindingTestPolicy{data: policy})
	pluginRoot := plugin_host_resource.NewPluginHostRoot(tb.Bus, "spacewave-core", "", nil, nil, nil,
		hostRoot, "test-policy", tb.Volume.GetID(), nil)
	t.Cleanup(pluginRoot.Release)
	hostMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot.GetMux()).Register(hostMux); err != nil {
		t.Fatal(err)
	}
	gen := waitSpaceRuntimeGeneration(t, runtime, nil)
	hostServer := bifrost_rpc.NewInvokerController(tb.Logger, gen.GetBus(),
		controller.NewInfo("test/worker-policy-server", controller.MustParseVersion("0.0.1"), ""),
		hostMux, nil)
	releaseServer, err := gen.GetBus().AddController(ctx, hostServer, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseServer)
}

// bindingTestBody presents the test World under one stable Space identity.
type bindingTestBody struct {
	// engine is the mounted Space World.
	engine world.Engine
	// engineID names that World to the plugin runtime.
	engineID string
	// ref identifies the mounted Space.
	ref *sobject.SharedObjectRef
}

// GetWorldEngine returns the mounted Space World.
func (b *bindingTestBody) GetWorldEngine() world.Engine { return b.engine }

// GetWorldEngineID returns the mounted World address.
func (b *bindingTestBody) GetWorldEngineID() string { return b.engineID }

// GetWorldEngineBucketID returns no forwarded bucket for this local World.
func (b *bindingTestBody) GetWorldEngineBucketID() string { return "" }

// GetSharedObjectRef returns the Space identity.
func (b *bindingTestBody) GetSharedObjectRef() *sobject.SharedObjectRef { return b.ref }

// GetSharedObject returns no remote SharedObject for this local fixture.
func (b *bindingTestBody) GetSharedObject() sobject.SharedObject { return nil }

// TestBindingWatchRetainsSharedRuntimeAfterCommandMountRelease checks that the
// independent binding watch sees approval, disable, and orphan deletion while
// only the keeper's reference keeps the shared runtime running.
func TestBindingWatchRetainsSharedRuntimeAfterCommandMountRelease(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	ref := &sobject.SharedObjectRef{}
	ref.ProviderResourceRef = &provider.ProviderResourceRef{}
	ref.ProviderResourceRef.ProviderId = "local"
	ref.ProviderResourceRef.ProviderAccountId = "test"
	ref.ProviderResourceRef.Id = "binding-watch"
	body := &bindingTestBody{engine: tb.Engine, engineID: tb.EngineID, ref: ref}
	spaceID := space.SpaceEngineId(ref)
	registry := process_binding.NewBindingRegistry()
	spaceResource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, tb.Volume.GetPeerID().String())
	spaceResource.SetBindingRegistry(registry)
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceResource.GetMux()))
	stream, err := client.WatchProcessBindings(ctx, &s4wave_space.WatchProcessBindingsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	initial, err := stream.Recv()
	if err != nil || len(initial.GetProcessBindings()) != 0 {
		t.Fatalf("initial binding snapshot = %#v, %v", initial, err)
	}

	conf := &plugin_space.Config{
		SpaceId: spaceID, VolumeId: tb.EngineVolumeID,
		ObjectStoreId: process_binding.DefaultObjectStoreID,
		EngineId:      tb.EngineID, SessionPeerId: tb.Volume.GetPeerID().String(),
	}
	mount := func() *SpaceContentsResource {
		t.Helper()
		runtime, runtimeRef, err := plugin_space_runtime.StartControllerWithConfig(
			ctx, tb.Bus, &plugin_space_runtime.Config{Space: conf},
		)
		if err != nil {
			t.Fatal(err)
		}
		contents := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, spaceID, tb.EngineID, runtime, runtimeRef)
		contents.volumeID = tb.EngineVolumeID
		contents.storeID = process_binding.DefaultObjectStoreID
		runtime.SetBindingRegistry(registry)
		return contents
	}
	createSpaceRuntimeObject(t, ctx, tb, "worker/test")
	command := mount()
	gen := waitSpaceRuntimeGeneration(t, command.runtime, nil)
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "worker/test", TypeId: forge_worker.WorkerTypeID, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}
	approved, err := stream.Recv()
	if err != nil || len(approved.GetProcessBindings()) != 1 || !approved.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("approved binding snapshot = %#v, %v", approved, err)
	}
	restored, err := client.WatchProcessBindings(ctx, &s4wave_space.WatchProcessBindingsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredSnapshot, err := restored.Recv()
	if err != nil || len(restoredSnapshot.GetProcessBindings()) != 1 || !restoredSnapshot.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("restored binding snapshot = %#v, %v", restoredSnapshot, err)
	}
	keeper := mount()
	if keeper.runtime != command.runtime {
		t.Fatal("keeper acquired a different Space runtime")
	}
	command.Release()
	select {
	case <-gen.Done():
		t.Fatal("command mount release stopped the retained runtime")
	default:
	}

	command = mount()
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "worker/test", TypeId: forge_worker.WorkerTypeID, Approved: false,
	}); err != nil {
		t.Fatal(err)
	}
	disabled, err := stream.Recv()
	if err != nil || len(disabled.GetProcessBindings()) != 1 || disabled.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("disabled binding snapshot = %#v, %v", disabled, err)
	}
	command.Release()
	keeper.Release()
	select {
	case <-gen.Done():
	case <-ctx.Done():
		t.Fatal("disabled binding retained the runtime")
	}

	command = mount()
	gen = waitSpaceRuntimeGeneration(t, command.runtime, nil)
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "worker/test", TypeId: forge_worker.WorkerTypeID, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}
	approved, err = stream.Recv()
	if err != nil || len(approved.GetProcessBindings()) != 1 || !approved.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("reapproved binding snapshot = %#v, %v", approved, err)
	}
	keeper = mount()
	command.Release()
	deleted, err := tb.WorldState.DeleteObject(ctx, "worker/test")
	if err != nil || !deleted {
		t.Fatalf("delete bound Worker = %v, %v", deleted, err)
	}
	orphaned, err := stream.Recv()
	if err != nil || len(orphaned.GetProcessBindings()) != 0 {
		t.Fatalf("orphan removal snapshot = %#v, %v", orphaned, err)
	}
	keeper.Release()
	select {
	case <-gen.Done():
	case <-ctx.Done():
		t.Fatal("orphan binding retained the runtime")
	}
}

// waitBindingWorkerCapacity follows World revisions until the Worker publishes
// the policy capacity after its process binding becomes active.
func waitBindingWorkerCapacity(t *testing.T, ctx context.Context, ws world.WorldState, workerKey string) *forge_runtime.WorkerCapacity {
	t.Helper()
	for {
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, workerKey)
		if err == nil && capacity.MilliCPUTotal != 0 {
			return capacity
		}
		if err != nil && err != forge_runtime.ErrWorkerNotObserved {
			t.Fatal(err)
		}
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatal(err)
		}
	}
}

// _ is a type assertion
var _ space.SpaceSharedObjectBody = (*bindingTestBody)(nil)
