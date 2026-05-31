//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

func TestTypedObjectResourceDevice(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	resClient, engine, cleanup := setupWorldResourceClientWithObjectTypes(ctx, t, tb)
	defer cleanup()

	objectKey := "devices/test-device"
	createdAt := timestamppb.New(time.Unix(100, 0))
	device := &s4wave_device.Device{
		PeerId:        "12D3KooWDevice",
		Label:         "Build Host",
		Platform:      &s4wave_device.DevicePlatform{Os: "linux", Arch: "arm64"},
		DaemonVersion: "test",
		SetupState:    s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		UpdateState:   s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE,
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    "ready",
			ObservedAt: createdAt.CloneVT(),
		},
		CreatedAt: createdAt.CloneVT(),
		UpdatedAt: createdAt.CloneVT(),
	}

	engineRef := resClient.CreateResourceReference(engine.GetResourceRef().GetResourceID())
	worldEngine, err := sdk_world_engine.NewSDKEngine(resClient, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatalf("NewSDKEngine failed: %v", err)
	}
	defer worldEngine.Release()

	tx, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}
	_, _, err = world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(device, true)
		return nil
	})
	if err != nil {
		tx.Discard()
		t.Fatalf("CreateWorldObject failed: %v", err)
	}
	if err := world_types.SetObjectType(ctx, tx, objectKey, s4wave_device.DeviceTypeID); err != nil {
		tx.Discard()
		t.Fatalf("SetObjectType failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		t.Fatalf("Commit failed: %v", err)
	}
	tx.Discard()

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read) failed: %v", err)
	}
	defer readTx.Release()

	srpcClient, err := readTx.GetResourceRef().GetClient()
	if err != nil {
		t.Fatalf("GetClient failed: %v", err)
	}
	typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
	resp, err := typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: objectKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject failed: %v", err)
	}
	if resp.GetTypeId() != s4wave_device.DeviceTypeID {
		t.Fatalf("type id = %q, want %q", resp.GetTypeId(), s4wave_device.DeviceTypeID)
	}

	deviceRef := resClient.CreateResourceReference(resp.GetResourceId())
	defer deviceRef.Release()
	deviceClient, err := deviceRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient(device) failed: %v", err)
	}
	deviceSvc := s4wave_device.NewSRPCDeviceResourceServiceClient(deviceClient)
	stream, err := deviceSvc.WatchDeviceState(ctx, &s4wave_device.WatchDeviceStateRequest{})
	if err != nil {
		t.Fatalf("WatchDeviceState failed: %v", err)
	}
	defer stream.Close()
	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv initial device state failed: %v", err)
	}
	if first.GetState().GetPeerId() != "12D3KooWDevice" {
		t.Fatalf("initial peer id = %q", first.GetState().GetPeerId())
	}

	if _, err := deviceSvc.ReportDeviceStatus(ctx, &s4wave_device.ReportDeviceStatusRequest{
		PeerId:      "12D3KooWDevice",
		UpdateState: s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY,
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_DEGRADED,
			Message:    "network limited",
			ObservedAt: timestamppb.New(time.Unix(120, 0)),
		},
		ReplaceCapabilities: true,
		Capabilities: []*s4wave_device.DeviceCapability{
			{
				Id:    "filesystem",
				Kind:  "filesystem",
				Label: "Files",
				Link: &s4wave_device.DeviceCapabilityLink{
					ObjectKey: "files/device-root",
					TypeId:    s4wave_unixfs_world.UnixFSTypeID,
				},
				Policy: &s4wave_device.DeviceCapabilityPolicy{
					LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
			{
				Id:    "forge-worker",
				Kind:  "forge-worker",
				Label: "Forge Worker",
				Link: &s4wave_device.DeviceCapabilityLink{
					ObjectKey: "forge/worker/device",
					TypeId:    forge_worker.WorkerTypeID,
				},
				Policy: &s4wave_device.DeviceCapabilityPolicy{
					LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
				},
			},
			{
				Id:    "terminal",
				Kind:  "terminal",
				Label: "Terminal",
				Link: &s4wave_device.DeviceCapabilityLink{
					ProtocolId: "alpha/remote-shell/v0",
				},
				Policy: &s4wave_device.DeviceCapabilityPolicy{
					LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_DISABLED,
					GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
		},
	}); err == nil {
		t.Fatal("expected browser-visible Device resource to reject status reports")
	}

	directUpdate := device.CloneVT()
	directUpdate.UpdateState = s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY
	directUpdate.LastStatus = &s4wave_device.DeviceStatus{
		Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_DEGRADED,
		Message:    "network limited",
		ObservedAt: timestamppb.New(time.Unix(120, 0)),
	}
	directUpdate.Capabilities = []*s4wave_device.DeviceCapability{
		{
			Id:    "filesystem",
			Kind:  "filesystem",
			Label: "Files",
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link: &s4wave_device.DeviceCapabilityLink{
				ObjectKey: "files/device-root",
				TypeId:    s4wave_unixfs_world.UnixFSTypeID,
			},
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
			},
		},
		{
			Id:     "forge-worker",
			Kind:   "forge-worker",
			Label:  "Forge Worker",
			State:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED,
			Detail: "blocked by Space grant",
			Link: &s4wave_device.DeviceCapabilityLink{
				ObjectKey: "forge/worker/device",
				TypeId:    forge_worker.WorkerTypeID,
			},
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
			},
		},
		{
			Id:     "terminal",
			Kind:   "terminal",
			Label:  "Terminal",
			State:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED,
			Detail: "disabled by local policy",
			Link: &s4wave_device.DeviceCapabilityLink{
				ProtocolId: "alpha/remote-shell/v0",
			},
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_DISABLED,
				GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
			},
		},
	}
	tx2, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(update) failed: %v", err)
	}
	updateState, found, err := tx2.GetObject(ctx, objectKey)
	if err != nil {
		tx2.Discard()
		t.Fatalf("GetObject(update) failed: %v", err)
	}
	if !found {
		tx2.Discard()
		t.Fatal("device object missing for direct update")
	}
	_, _, err = world.AccessObjectState(ctx, updateState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(directUpdate, true)
		return nil
	})
	if err != nil {
		tx2.Discard()
		t.Fatalf("AccessObjectState(update) failed: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		tx2.Discard()
		t.Fatalf("Commit(update) failed: %v", err)
	}
	tx2.Discard()

	caps := directUpdate.GetCapabilities()
	if len(caps) != 3 {
		t.Fatalf("capabilities = %d, want 3", len(caps))
	}
	if caps[0].GetState() != s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE {
		t.Fatalf("filesystem state = %v, want available", caps[0].GetState())
	}
	if caps[0].GetLink().GetObjectKey() != "files/device-root" || caps[0].GetLink().GetTypeId() != s4wave_unixfs_world.UnixFSTypeID {
		t.Fatalf("filesystem link = %#v", caps[0].GetLink())
	}
	if caps[1].GetState() != s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED {
		t.Fatalf("forge state = %v, want grant blocked", caps[1].GetState())
	}
	if caps[1].GetLink().GetObjectKey() != "forge/worker/device" || caps[1].GetLink().GetTypeId() != forge_worker.WorkerTypeID {
		t.Fatalf("forge link = %#v", caps[1].GetLink())
	}
	if caps[2].GetState() != s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED {
		t.Fatalf("terminal state = %v, want disabled", caps[2].GetState())
	}
	if caps[2].GetLink().GetProtocolId() != "alpha/remote-shell/v0" {
		t.Fatalf("terminal protocol = %q", caps[2].GetLink().GetProtocolId())
	}

	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	gotUpdate, err := recvDeviceState(watchCtx, stream)
	if err != nil {
		t.Fatalf("recv updated device state: %v", err)
	}
	if gotUpdate.GetLastStatus().GetMessage() != "network limited" {
		t.Fatalf("watch status = %q", gotUpdate.GetLastStatus().GetMessage())
	}
}

func TestDeviceResourceAccessCheckoutRoot(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	resClient, engine, cleanup := setupWorldResourceClientWithObjectTypes(ctx, t, tb)
	defer cleanup()

	engineRef := resClient.CreateResourceReference(engine.GetResourceRef().GetResourceID())
	worldEngine, err := sdk_world_engine.NewSDKEngine(resClient, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatalf("NewSDKEngine failed: %v", err)
	}
	defer worldEngine.Release()

	tx, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}

	fsObjectKey := "unixfs/skiffos-checkout"
	if _, _, err := space_world_ops.InitUnixFS(ctx, tx, tb.Volume.GetPeerID(), fsObjectKey, time.Unix(100, 0)); err != nil {
		tx.Discard()
		t.Fatalf("InitUnixFS failed: %v", err)
	}

	deviceObjectKey := "devices/lima"
	device := &s4wave_device.Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  s4wave_device.DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link: &s4wave_device.DeviceCapabilityLink{
				ObjectKey: fsObjectKey,
				TypeId:    s4wave_unixfs_world.UnixFSTypeID,
			},
			CheckoutRoot: &s4wave_device.DeviceCheckoutRootCapability{
				Name:           "skiffos",
				DisplayPath:    "~/repos/skiffos/skiffos",
				SelectionRef:   "device/lima/filesystem/skiffos",
				Access:         s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE,
				ReadAvailable:  true,
				WriteAvailable: true,
			},
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
			},
		}},
	}
	_, _, err = world.CreateWorldObject(ctx, tx, deviceObjectKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(device, true)
		return nil
	})
	if err != nil {
		tx.Discard()
		t.Fatalf("CreateWorldObject(device) failed: %v", err)
	}
	if err := world_types.SetObjectType(ctx, tx, deviceObjectKey, s4wave_device.DeviceTypeID); err != nil {
		tx.Discard()
		t.Fatalf("SetObjectType(device) failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		t.Fatalf("Commit failed: %v", err)
	}
	tx.Discard()

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read) failed: %v", err)
	}
	defer readTx.Release()

	srpcClient, err := readTx.GetResourceRef().GetClient()
	if err != nil {
		t.Fatalf("GetClient(readTx) failed: %v", err)
	}
	typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
	deviceResp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: deviceObjectKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject(device) failed: %v", err)
	}

	deviceRef := resClient.CreateResourceReference(deviceResp.GetResourceId())
	defer deviceRef.Release()
	deviceClient, err := deviceRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient(device) failed: %v", err)
	}
	deviceSvc := s4wave_device.NewSRPCDeviceResourceServiceClient(deviceClient)
	checkoutResp, err := deviceSvc.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{Name: "skiffos"})
	if err != nil {
		t.Fatalf("AccessCheckoutRoot failed: %v", err)
	}
	if checkoutResp.GetObjectKey() != fsObjectKey {
		t.Fatalf("checkout object key = %q, want %q", checkoutResp.GetObjectKey(), fsObjectKey)
	}
	if checkoutResp.GetTypeId() != s4wave_unixfs_world.UnixFSTypeID {
		t.Fatalf("checkout type id = %q", checkoutResp.GetTypeId())
	}
	if !checkoutResp.GetWriteAvailable() {
		t.Fatal("expected checkout root to report guarded write availability")
	}
	if checkoutResp.GetWriteEnabled() {
		t.Fatal("read request returned write-enabled checkout root")
	}

	fsRef := resClient.CreateResourceReference(checkoutResp.GetResourceId())
	fsClient, err := fsRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient(checkout root) failed: %v", err)
	}
	fsSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fsClient)
	nodeType, err := fsSvc.GetNodeType(ctx, &s4wave_unixfs.HandleGetNodeTypeRequest{})
	if err != nil {
		t.Fatalf("GetNodeType(checkout root) failed: %v", err)
	}
	if !nodeType.GetNodeType().GetIsDir() {
		t.Fatal("expected checkout root to resolve as directory")
	}
	if _, err := fsSvc.Mknod(ctx, &s4wave_unixfs.HandleMknodRequest{
		Names:      []string{"should-not-write.txt"},
		NodeType:   s4wave_unixfs.MknodType_MKNOD_TYPE_FILE,
		CheckExist: true,
	}); err == nil {
		t.Fatal("expected read-only checkout root to reject Mknod")
	}

	watchCtx, cancelWatch := context.WithTimeout(ctx, 5*time.Second)
	defer cancelWatch()
	watch, err := fsSvc.WatchReaddir(watchCtx, &s4wave_unixfs.HandleWatchReaddirRequest{})
	if err != nil {
		t.Fatalf("WatchReaddir(checkout root) failed: %v", err)
	}
	initialEntries, err := watch.Recv()
	if err != nil {
		t.Fatalf("WatchReaddir initial recv failed: %v", err)
	}
	if hasDirEntry(initialEntries.GetEntries(), "write-ok.txt") {
		t.Fatal("initial read checkout watch unexpectedly saw future write")
	}

	if _, err := deviceSvc.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{
		Name:  "skiffos",
		Write: true,
	}); err == nil {
		t.Fatal("expected write checkout root access to require approval ref")
	}

	writeResp, err := deviceSvc.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{
		Name:             "skiffos",
		Write:            true,
		WriteApprovalRef: "decision/test-write-approved",
	})
	if err != nil {
		t.Fatalf("write AccessCheckoutRoot failed: %v", err)
	}
	if !writeResp.GetWriteEnabled() {
		t.Fatal("expected write checkout root response to report write enabled")
	}
	if writeResp.GetWriteApprovalRef() != "decision/test-write-approved" {
		t.Fatalf("write approval ref = %q", writeResp.GetWriteApprovalRef())
	}

	writeFSRef := resClient.CreateResourceReference(writeResp.GetResourceId())
	defer writeFSRef.Release()
	writeFSClient, err := writeFSRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient(write checkout root) failed: %v", err)
	}
	writeFSSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(writeFSClient)
	if _, err := writeFSSvc.Mknod(ctx, &s4wave_unixfs.HandleMknodRequest{
		Names:      []string{"write-ok.txt"},
		NodeType:   s4wave_unixfs.MknodType_MKNOD_TYPE_FILE,
		CheckExist: true,
	}); err != nil {
		t.Fatalf("expected approved write checkout root to allow Mknod: %v", err)
	}
	updatedEntries, err := watch.Recv()
	if err != nil {
		t.Fatalf("WatchReaddir update recv failed: %v", err)
	}
	if !hasDirEntry(updatedEntries.GetEntries(), "write-ok.txt") {
		t.Fatalf("checkout watch did not observe write-ok.txt: %v", updatedEntries.GetEntries())
	}
	cancelWatch()
	fsRef.Release()
	releasedFSRef := resClient.CreateResourceReference(checkoutResp.GetResourceId())
	defer releasedFSRef.Release()
	releasedFSClient, err := releasedFSRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient(released checkout root) failed: %v", err)
	}
	releasedFSSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(releasedFSClient)
	if _, err := releasedFSSvc.GetNodeType(ctx, &s4wave_unixfs.HandleGetNodeTypeRequest{}); err == nil {
		t.Fatal("expected released checkout root resource to reject GetNodeType")
	}

	revokeTx, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(revoke) failed: %v", err)
	}
	revokeState, found, err := revokeTx.GetObject(ctx, deviceObjectKey)
	if err != nil {
		revokeTx.Discard()
		t.Fatalf("GetObject(revoke) failed: %v", err)
	}
	if !found {
		revokeTx.Discard()
		t.Fatal("device object missing for revoke")
	}
	revokedDevice := device.CloneVT()
	revokedDevice.Capabilities[0].Policy.GrantState = s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED
	_, _, err = world.AccessObjectState(ctx, revokeState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(revokedDevice, true)
		return nil
	})
	if err != nil {
		revokeTx.Discard()
		t.Fatalf("AccessObjectState(revoke) failed: %v", err)
	}
	if err := revokeTx.Commit(ctx); err != nil {
		revokeTx.Discard()
		t.Fatalf("Commit(revoke) failed: %v", err)
	}
	revokeTx.Discard()

	if _, err := deviceSvc.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{
		Name:             "skiffos",
		Write:            true,
		WriteApprovalRef: "decision/stale-write-approved",
	}); err == nil {
		t.Fatal("expected stale Device resource to reject write after policy revoke")
	}

	disabledTx, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(disable) failed: %v", err)
	}
	disabledState, found, err := disabledTx.GetObject(ctx, deviceObjectKey)
	if err != nil {
		disabledTx.Discard()
		t.Fatalf("GetObject(disable) failed: %v", err)
	}
	if !found {
		disabledTx.Discard()
		t.Fatal("device object missing for disable")
	}
	disabledDevice := device.CloneVT()
	disabledDevice.Capabilities[0].State = s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED
	_, _, err = world.AccessObjectState(ctx, disabledState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(disabledDevice, true)
		return nil
	})
	if err != nil {
		disabledTx.Discard()
		t.Fatalf("AccessObjectState(disable) failed: %v", err)
	}
	if err := disabledTx.Commit(ctx); err != nil {
		disabledTx.Discard()
		t.Fatalf("Commit(disable) failed: %v", err)
	}
	disabledTx.Discard()

	disabledResp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: deviceObjectKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject(disabled device) failed: %v", err)
	}
	disabledRef := resClient.CreateResourceReference(disabledResp.GetResourceId())
	defer disabledRef.Release()
	disabledClient, err := disabledRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient(disabled device) failed: %v", err)
	}
	disabledSvc := s4wave_device.NewSRPCDeviceResourceServiceClient(disabledClient)
	if _, err := disabledSvc.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{Name: "skiffos"}); err == nil {
		t.Fatal("expected disabled checkout root to reject read access")
	}
}

func hasDirEntry(entries []*s4wave_unixfs.DirEntry, name string) bool {
	for _, entry := range entries {
		if entry.GetName() == name {
			return true
		}
	}
	return false
}

func recvDeviceState(ctx context.Context, stream s4wave_device.SRPCDeviceResourceService_WatchDeviceStateClient) (*s4wave_device.Device, error) {
	type result struct {
		resp *s4wave_device.WatchDeviceStateResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := stream.Recv()
		ch <- result{resp: resp, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.resp.GetState(), nil
	}
}
