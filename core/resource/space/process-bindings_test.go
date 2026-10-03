package resource_space

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
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

// addBindingTestWorkerPolicyHost publishes the native host Root with a bound
// policy source on the daemon bus, as the native daemon does.
func addBindingTestWorkerPolicyHost(t *testing.T, ctx context.Context, tb *testbed.Testbed, workerKey string) {
	// Bind one policy revision to a fresh host Root.
	t.Helper()
	policy, err := (&device_policy.DevicePolicy{Revision: 1, ForgeWorker: &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: workerKey, MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{"docker"},
	}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	hostRoot := plugin_host_root.NewRoot()
	hostRoot.SetDevicePolicySource(&bindingTestPolicy{data: policy})

	// Resolve native host Root lookups with that Root.
	platformID := (&bldr_platform.NativePlatform{}).GetPlatformID()
	rootCtrl := callback.NewCallbackController(
		controller.NewInfo("test/native-host-root", controller.MustParseVersion("0.0.1"), ""),
		nil,
		func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			d, ok := inst.GetDirective().(plugin_host_root.LookupRoot)
			if !ok {
				return nil, nil
			}
			if ids := d.LookupRootPlatformIDs(); len(ids) != 0 && !slices.Contains(ids, platformID) {
				return nil, nil
			}
			return directive.R(directive.NewValueResolver([]plugin_host_root.LookupRootValue{hostRoot}), nil)
		},
		nil,
	)
	releaseRoot, err := tb.Bus.AddController(ctx, rootCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseRoot)
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
	// Prepare the World and stable Space identity for process binding watches.
	ctx, tb := newSpaceRuntimeTestbed(t)
	ref := &sobject.SharedObjectRef{}
	ref.ProviderResourceRef = &provider.ProviderResourceRef{}
	ref.ProviderResourceRef.ProviderId = "local"
	ref.ProviderResourceRef.ProviderAccountId = "test"
	ref.ProviderResourceRef.Id = "binding-watch"
	body := &bindingTestBody{engine: tb.Engine, engineID: tb.EngineID, ref: ref}
	spaceID := space.SpaceEngineId(ref)

	// Watch local process decisions independently of a contents mount.
	registry := process_binding.NewBindingRegistry()
	spaceResource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, tb.Volume.GetPeerID().String())
	spaceResource.SetBindingRegistry(registry)
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceResource.GetMux()))
	stream, err := client.WatchProcessBindings(ctx, &s4wave_space.WatchProcessBindingsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Verify the independent binding watch starts with no process decisions.
	initial, err := stream.Recv()
	if err != nil || len(initial.GetProcessBindings()) != 0 {
		t.Fatalf("initial binding snapshot = %#v, %v", initial, err)
	}

	// Prepare contents mounts that share the Space runtime and binding registry.
	conf := &plugin_space.Config{
		SpaceId: spaceID, VolumeId: tb.EngineVolumeID,
		ObjectStoreId: process_binding.DefaultObjectStoreID,
		EngineId:      tb.EngineID, SessionPeerId: tb.Volume.GetPeerID().String(),
	}
	mount := func() *SpaceContentsResource {
		// Start the shared runtime for a contents mount.
		t.Helper()
		runtime, runtimeRef, err := plugin_space_runtime.StartControllerWithConfig(
			ctx, tb.Bus, &plugin_space_runtime.Config{Space: conf},
		)
		if err != nil {
			t.Fatal(err)
		}

		// Connect the contents mount to the local process decision store.
		contents := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, spaceID, tb.EngineID, runtime, runtimeRef)
		contents.volumeID = tb.EngineVolumeID
		contents.storeID = process_binding.DefaultObjectStoreID
		runtime.SetBindingRegistry(registry)
		return contents
	}

	// Approve the Worker binding through a command contents mount.
	createSpaceRuntimeObject(t, ctx, tb, "worker/test")
	command := mount()
	gen := waitSpaceRuntimeGeneration(t, command.runtime, nil)
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "worker/test", TypeId: forge_worker.WorkerTypeID, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Verify the binding watch reports the approved Worker.
	approved, err := stream.Recv()
	if err != nil || len(approved.GetProcessBindings()) != 1 || !approved.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("approved binding snapshot = %#v, %v", approved, err)
	}

	// Restore a binding watch and verify it retains the saved approval.
	restored, err := client.WatchProcessBindings(ctx, &s4wave_space.WatchProcessBindingsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredSnapshot, err := restored.Recv()
	if err != nil || len(restoredSnapshot.GetProcessBindings()) != 1 || !restoredSnapshot.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("restored binding snapshot = %#v, %v", restoredSnapshot, err)
	}

	// Release the command mount while a keeper retains its runtime.
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

	// Disable the Worker binding through a new command mount.
	command = mount()
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "worker/test", TypeId: forge_worker.WorkerTypeID, Approved: false,
	}); err != nil {
		t.Fatal(err)
	}

	// Verify the binding watch reports the disabled Worker.
	disabled, err := stream.Recv()
	if err != nil || len(disabled.GetProcessBindings()) != 1 || disabled.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("disabled binding snapshot = %#v, %v", disabled, err)
	}

	// Release both mounts and verify the disabled Worker retains no runtime.
	command.Release()
	keeper.Release()
	select {
	case <-gen.Done():
	case <-ctx.Done():
		t.Fatal("disabled binding retained the runtime")
	}

	// Approve the Worker again in a new runtime generation.
	command = mount()
	gen = waitSpaceRuntimeGeneration(t, command.runtime, nil)
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "worker/test", TypeId: forge_worker.WorkerTypeID, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Verify the binding watch reports the restored Worker approval.
	approved, err = stream.Recv()
	if err != nil || len(approved.GetProcessBindings()) != 1 || !approved.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("reapproved binding snapshot = %#v, %v", approved, err)
	}

	// Delete the bound Worker while a keeper retains the runtime.
	keeper = mount()
	command.Release()
	deleted, err := tb.WorldState.DeleteObject(ctx, "worker/test")
	if err != nil || !deleted {
		t.Fatalf("delete bound Worker = %v, %v", deleted, err)
	}

	// Verify the binding watch removes the deleted Worker decision.
	orphaned, err := stream.Recv()
	if err != nil || len(orphaned.GetProcessBindings()) != 0 {
		t.Fatalf("orphan removal snapshot = %#v, %v", orphaned, err)
	}

	// Release the keeper and verify the orphaned binding retains no runtime.
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
