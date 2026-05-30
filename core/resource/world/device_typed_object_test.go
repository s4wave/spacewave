//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
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
		PeerId: "12D3KooWOther",
		LastStatus: &s4wave_device.DeviceStatus{
			Message: "wrong peer",
		},
	}); err == nil {
		t.Fatal("expected mismatched peer_id to fail status report")
	}

	updateResp, err := deviceSvc.ReportDeviceStatus(ctx, &s4wave_device.ReportDeviceStatusRequest{
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
				State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			},
		},
	})
	if err != nil {
		t.Fatalf("ReportDeviceStatus failed: %v", err)
	}
	if updateResp.GetState().GetUpdateState() != s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY {
		t.Fatalf("update state = %v", updateResp.GetState().GetUpdateState())
	}
	if len(updateResp.GetState().GetCapabilities()) != 1 {
		t.Fatalf("capabilities = %d, want 1", len(updateResp.GetState().GetCapabilities()))
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
