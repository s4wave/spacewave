package resource_space

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// TestQueueSpacePluginBuild checks the public RPC's device binding and immutable input.
func TestQueueSpacePluginBuild(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	request := setupPluginBuildSource(t, tb, tb.Volume.GetPeerID())
	peer := tb.Volume.GetPeerID()
	root, _, err := world.LookupRootRef(ctx, tb.Engine, request.GetSourceKey())
	if err != nil {
		t.Fatal(err)
	}
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peer.String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	response, err := client.BuildSpacePlugin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	execution, object, err := world.LookupObject[*forge_execution.Execution](ctx, tb.WorldState,
		response.GetExecutionKey(), forge_execution.NewExecutionBlock)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := execution.GetValueSet().LookupInput("source")
	if execution.GetPeerId() != peer.String() || !input.GetWorldObjectSnapshot().GetRootRef().EqualVT(root) {
		t.Fatal("queued execution lost its selected device or exact source")
	}

	// Invalid selection and anonymous submissions cannot enqueue executable work.
	request.DeviceKey = request.SourceKey
	if _, err := client.BuildSpacePlugin(ctx, request); err == nil {
		t.Fatal("accepted a non-device build target")
	}
	anonymous := NewSpaceResource(tb.Logger, tb.Bus, body)
	if _, err := anonymous.BuildSpacePlugin(ctx, request); err == nil {
		t.Fatal("accepted an anonymous build")
	}
}

// setupPluginBuildSource creates a source tree and a granted local build device.
func setupPluginBuildSource(t *testing.T, tb *testbed.Testbed, peer peer.ID) *s4wave_space.BuildSpacePluginRequest {
	t.Helper()
	ctx := t.Context()
	const sourceKey = "projects/colors"
	const deviceKey = "devices/build"
	const workerKey = "workers/build"
	_, _, err := unixfs_world.FsInit(ctx, tb.WorldState, peer, sourceKey,
		unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := peer.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(publicKey, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = forge_worker.CreateWorker(ctx, tb.WorldState, workerKey, "build",
		[]*identity.Keypair{keypair}, peer)
	if err != nil {
		t.Fatal(err)
	}
	device := &s4wave_device.Device{
		PeerId: peer.String(), Label: "Build device",
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
	_, _, err = world.AccessWorldObject(ctx, tb.WorldState, deviceKey, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(device, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, deviceKey, s4wave_device.DeviceTypeID); err != nil {
		t.Fatal(err)
	}

	return &s4wave_space.BuildSpacePluginRequest{
		SourceKey: sourceKey, DeviceKey: deviceKey, ManifestId: "space-colors",
	}
}
