//go:build !js

package spacewave_cli

import (
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func TestProjectLauncherUpdateOntoDeviceLifecycle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	created := timestamppb.New(now.Add(-time.Hour))
	updated := timestamppb.New(now.Add(-time.Minute))
	base := &s4wave_device.Device{
		PeerId:        "peer-device",
		Label:         "build host",
		Platform:      &s4wave_device.DevicePlatform{Os: "linux", Arch: "amd64"},
		DaemonVersion: "unknown",
		SetupState:    s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		UpdateState:   s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE,
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    "device session ready",
			ObservedAt: updated.CloneVT(),
		},
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id:    "terminal",
			Kind:  "terminal",
			Label: "Terminal",
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED,
		}},
		CreatedAt: created.CloneVT(),
		UpdatedAt: updated.CloneVT(),
	}

	tests := []struct {
		name         string
		existing     *s4wave_device.Device
		info         *spacewave_launcher.LauncherInfo
		wantChanged  bool
		wantState    s4wave_device.DeviceUpdateState
		wantLive     s4wave_device.DeviceLiveness
		wantMessage  string
		wantError    string
		wantUpdateAt bool
	}{
		{
			name:        "idle is idempotent",
			existing:    base.CloneVT(),
			info:        &spacewave_launcher.LauncherInfo{},
			wantChanged: false,
			wantState:   s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE,
			wantLive:    s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			wantMessage: "device session ready",
		},
		{
			name:         "staging update",
			info:         launcherInfo(spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING, "0.2.0", ""),
			wantChanged:  true,
			wantState:    s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_STAGING,
			wantLive:     s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			wantMessage:  "staging update: 0.2.0",
			wantUpdateAt: true,
		},
		{
			name:         "ready to apply",
			info:         launcherInfo(spacewave_launcher.UpdatePhase_UpdatePhase_STAGED, "0.2.0", ""),
			wantChanged:  true,
			wantState:    s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY,
			wantLive:     s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			wantMessage:  "update ready: 0.2.0",
			wantUpdateAt: true,
		},
		{
			name:         "applying update",
			info:         launcherInfo(spacewave_launcher.UpdatePhase_UpdatePhase_APPLYING, "0.2.0", ""),
			wantChanged:  true,
			wantState:    s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_APPLYING,
			wantLive:     s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			wantMessage:  "applying update: 0.2.0",
			wantUpdateAt: true,
		},
		{
			name:         "failed staging remains visible",
			info:         launcherInfo(spacewave_launcher.UpdatePhase_UpdatePhase_ERROR, "", "checkout release manifest: denied"),
			wantChanged:  true,
			wantState:    s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_FAILED,
			wantLive:     s4wave_device.DeviceLiveness_DEVICE_LIVENESS_DEGRADED,
			wantMessage:  "update failed",
			wantError:    "checkout release manifest: denied",
			wantUpdateAt: true,
		},
		{
			name:         "failed apply remains visible",
			info:         launcherInfo(spacewave_launcher.UpdatePhase_UpdatePhase_ERROR, "", "stat staged path: missing"),
			wantChanged:  true,
			wantState:    s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_FAILED,
			wantLive:     s4wave_device.DeviceLiveness_DEVICE_LIVENESS_DEGRADED,
			wantMessage:  "update failed",
			wantError:    "stat staged path: missing",
			wantUpdateAt: true,
		},
		{
			name: "successful relaunch from applying",
			existing: func() *s4wave_device.Device {
				d := base.CloneVT()
				d.UpdateState = s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_APPLYING
				d.LastStatus = &s4wave_device.DeviceStatus{
					Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
					Message:    "applying update: 0.2.0",
					ObservedAt: updated.CloneVT(),
				}
				return d
			}(),
			info:         &spacewave_launcher.LauncherInfo{},
			wantChanged:  true,
			wantState:    s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_UPDATED,
			wantLive:     s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			wantMessage:  "update applied",
			wantUpdateAt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := tt.existing
			if existing == nil {
				existing = base.CloneVT()
			}
			next, changed, err := projectLauncherUpdateOntoDevice(existing, tt.info, now)
			if err != nil {
				t.Fatalf("projectLauncherUpdateOntoDevice() error = %v", err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if next.GetPeerId() != existing.GetPeerId() || next.GetLabel() != existing.GetLabel() {
				t.Fatalf("identity changed: got %q/%q want %q/%q", next.GetPeerId(), next.GetLabel(), existing.GetPeerId(), existing.GetLabel())
			}
			if next.GetSetupState() != existing.GetSetupState() {
				t.Fatalf("setup state changed: got %v want %v", next.GetSetupState(), existing.GetSetupState())
			}
			if next.GetCreatedAt().GetSeconds() != created.GetSeconds() {
				t.Fatalf("created_at changed: got %v want %v", next.GetCreatedAt(), created)
			}
			if len(next.GetCapabilities()) != len(existing.GetCapabilities()) {
				t.Fatalf("capabilities changed: got %d want %d", len(next.GetCapabilities()), len(existing.GetCapabilities()))
			}
			if next.GetUpdateState() != tt.wantState {
				t.Fatalf("update state = %v, want %v", next.GetUpdateState(), tt.wantState)
			}
			status := next.GetLastStatus()
			if status.GetLiveness() != tt.wantLive {
				t.Fatalf("liveness = %v, want %v", status.GetLiveness(), tt.wantLive)
			}
			if status.GetMessage() != tt.wantMessage {
				t.Fatalf("message = %q, want %q", status.GetMessage(), tt.wantMessage)
			}
			if status.GetError() != tt.wantError {
				t.Fatalf("error = %q, want %q", status.GetError(), tt.wantError)
			}
			gotUpdatedAt := next.GetUpdatedAt().GetSeconds() == now.Unix()
			if gotUpdatedAt != tt.wantUpdateAt {
				t.Fatalf("updated_at at projection time = %v, want %v", gotUpdatedAt, tt.wantUpdateAt)
			}
		})
	}
}

func TestDeviceLauncherProjectionTargetRequiresReadyDeviceRecord(t *testing.T) {
	statePath := t.TempDir()
	if _, ok, err := deviceLauncherProjectionTarget(statePath); err != nil || ok {
		t.Fatalf("missing setup target = (%v, %v), want no target without error", ok, err)
	}

	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{
		SetupState:      deviceSetupStateSessionReady,
		PeerID:          "peer-device",
		ResourceID:      "c3BhY2UtMQ==",
		SessionIndex:    7,
		DeviceObjectKey: "devices/build-host",
	}); err != nil {
		t.Fatalf("write setup record: %v", err)
	}
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil {
		t.Fatalf("deviceLauncherProjectionTarget() error = %v", err)
	}
	if !ok {
		t.Fatal("ready device setup record was not selected as projection target")
	}
	if record.DeviceObjectKey != "devices/build-host" {
		t.Fatalf("device object key = %q", record.DeviceObjectKey)
	}
}

func launcherInfo(phase spacewave_launcher.UpdatePhase, version, errMessage string) *spacewave_launcher.LauncherInfo {
	return &spacewave_launcher.LauncherInfo{
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:        phase,
			Version:      version,
			ErrorMessage: errMessage,
			StagedPath:   "/staged/spacewave",
		},
	}
}
