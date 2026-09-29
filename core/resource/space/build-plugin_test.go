package resource_space

import (
	"testing"
	"time"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// TestQueueSpacePluginBuild checks the public RPC's Job, device binding, and immutable input.
func TestQueueSpacePluginBuild(t *testing.T) {
	// Start a testbed with a build device and pin its source root.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	peer := tb.Volume.GetPeerID()
	request := setupPluginBuildSource(t, tb, peer)
	root, _, err := world.LookupRootRef(ctx, tb.Engine, request.GetSourceKey())
	if err != nil {
		t.Fatal(err)
	}

	// Mount the Space Resource as the submitting Session and queue the build.
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peer.String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	response, err := client.BuildSpacePlugin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	// The Job carries the selected Worker and peer and belongs to the Cluster.
	job, err := forge_job.LookupJobBody(ctx, tb.WorldState, response.GetJobKey())
	if err != nil {
		t.Fatal(err)
	}
	if job.GetPlacement().GetWorkerObjectKey() != "workers/build" || job.GetPlacement().GetPeerId() != peer.String() {
		t.Fatal("queued job lost its selected device")
	}
	assigned, err := forge_cluster.CheckClusterHasJob(ctx, tb.WorldState, "clusters/build", response.GetJobKey())
	if err != nil || !assigned {
		t.Fatalf("job is not assigned to its cluster: %v", err)
	}

	// The single Task inherits the placement and pins the exact source.
	task, err := forge_task.LookupTaskBody(ctx, tb.WorldState, response.GetTaskKey())
	if err != nil {
		t.Fatal(err)
	}
	if !task.GetPlacement().EqualVT(job.GetPlacement()) {
		t.Fatal("queued task lost its selected device")
	}
	target, _, err := forge_task.LookupTaskTarget(ctx, tb.WorldState, response.GetTaskKey())
	if err != nil {
		t.Fatal(err)
	}
	input := target.GetInputs()[0].GetValue().GetWorldObjectSnapshot()
	if target.GetInputs()[0].GetName() != "source" || !input.GetRootRef().EqualVT(root) {
		t.Fatal("queued task did not pin the exact source")
	}

	// The Task config carries the requested platform and capacity.
	var config space_exec.PluginBuildConfig
	if err := config.UnmarshalJSON(target.GetExec().GetController().GetConfig()); err != nil {
		t.Fatal(err)
	}
	if config.GetPlatformId() != "desktop/linux/amd64" || config.GetMilliCpu() != 1000 || config.GetMemoryBytes() != 1<<30 {
		t.Fatalf("queued task config lost its platform or capacity: %v", &config)
	}

	// Invalid selections and anonymous submissions cannot enqueue executable work.
	assertPluginBuildRejected(t, client, request, "a non-device build target", func(bad *s4wave_space.BuildSpacePluginRequest) {
		bad.DeviceKey = request.SourceKey
	})
	assertPluginBuildRejected(t, client, request, "a platform the device cannot run", func(bad *s4wave_space.BuildSpacePluginRequest) {
		bad.PlatformId = "desktop/darwin/arm64"
	})
	assertPluginBuildRejected(t, client, request, "a build without an explicit capacity request", func(bad *s4wave_space.BuildSpacePluginRequest) {
		bad.MemoryBytes = 0
	})
	assertPluginBuildRejected(t, client, request, "a build the worker cannot hold", func(bad *s4wave_space.BuildSpacePluginRequest) {
		bad.MilliCpu = 1 << 40
	})
	assertPluginBuildRejected(t, client, request, "a cluster without the selected worker", func(bad *s4wave_space.BuildSpacePluginRequest) {
		bad.ClusterKey = "clusters/other"
	})
	anonymous := NewSpaceResource(tb.Logger, tb.Bus, body)
	if _, err := anonymous.BuildSpacePlugin(ctx, request); err == nil {
		t.Fatal("accepted an anonymous build")
	}
}

// assertPluginBuildRejected checks the RPC rejects a copy of request altered by mutate.
func assertPluginBuildRejected(t *testing.T, client s4wave_space.SRPCSpaceResourceServiceClient, request *s4wave_space.BuildSpacePluginRequest, what string, mutate func(*s4wave_space.BuildSpacePluginRequest)) {
	// Alter a copy so the shared request stays valid.
	t.Helper()
	bad := request.CloneVT()
	mutate(bad)

	// Submit it and require an error.
	if _, err := client.BuildSpacePlugin(t.Context(), bad); err == nil {
		t.Fatalf("accepted %s", what)
	}
}

// setupPluginBuildSource creates a source tree and a granted local build device.
func setupPluginBuildSource(t *testing.T, tb *testbed.Testbed, peer peer.ID) *s4wave_space.BuildSpacePluginRequest {
	// Create the empty source directory.
	t.Helper()
	ctx := t.Context()
	const sourceKey = "projects/colors"
	const deviceKey = "devices/build"
	const workerKey = "workers/build"
	const clusterKey = "clusters/build"
	_, _, err := unixfs_world.FsInit(ctx, tb.WorldState, peer, sourceKey,
		unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Register the Device, its Worker, and the Cluster that groups the Worker.
	createPluginBuildWorker(t, tb, peer, workerKey)
	writePluginBuildDevice(t, tb, peer, deviceKey, workerKey)
	if _, _, err := forge_cluster.CreateCluster(ctx, tb.WorldState, clusterKey, "build", peer, peer); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.AssignWorkerToCluster(ctx, tb.WorldState, clusterKey, workerKey, peer); err != nil {
		t.Fatal(err)
	}

	// Observe the Worker's capacity for the submit-time check.
	admission := forge_runtime.NewWorldRuntimeAdmission(tb.Engine, nil, 0, 0)
	ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: deviceKey, ClaimID: "build"}
	claimed, err := admission.ClaimWorkerCapacity(ctx, workerKey, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, workerKey, ref, claimed.OwnerEpoch, 4000, 8<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	return &s4wave_space.BuildSpacePluginRequest{
		SourceKey: sourceKey, DeviceKey: deviceKey, ManifestId: "space-colors",
		PlatformId: "desktop/linux/amd64", MilliCpu: 1000, MemoryBytes: 1 << 30,
	}
}

// createPluginBuildWorker creates a Forge Worker owned by the peer's keypair.
func createPluginBuildWorker(t *testing.T, tb *testbed.Testbed, peer peer.ID, workerKey string) {
	// Derive the keypair that owns the Worker's assignments.
	t.Helper()
	publicKey, err := peer.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(publicKey, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create the Worker in the World.
	_, _, err = forge_worker.CreateWorker(t.Context(), tb.WorldState, workerKey, "build",
		[]*identity.Keypair{keypair}, peer)
	if err != nil {
		t.Fatal(err)
	}
}

// writePluginBuildDevice stores a linux/amd64 Device with an available Forge
// Worker capability linked to workerKey.
func writePluginBuildDevice(t *testing.T, tb *testbed.Testbed, peer peer.ID, deviceKey, workerKey string) {
	// Describe a ready Device whose Worker capability is enabled and granted.
	t.Helper()
	ctx := t.Context()
	device := &s4wave_device.Device{
		PeerId: peer.String(), Label: "Build device",
		Platform:   &s4wave_device.DevicePlatform{Os: "linux", Arch: "amd64"},
		SetupState: s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id: "worker", Kind: s4wave_device.DeviceCapabilityKindForgeWorker,
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link:  &s4wave_device.DeviceCapabilityLink{ObjectKey: workerKey, TypeId: forge_worker.WorkerTypeID},
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "build/worker", GrantPolicyRef: "space/build/worker",
				LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
			},
		}},
	}

	// Write the Device object and its type.
	_, _, err := world.AccessWorldObject(ctx, tb.WorldState, deviceKey, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(device, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, deviceKey, s4wave_device.DeviceTypeID); err != nil {
		t.Fatal(err)
	}
}
