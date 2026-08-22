package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	device_policy "github.com/s4wave/spacewave/core/device/policy"

	"github.com/s4wave/spacewave/core/device/sensor"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

// TestMountDeviceSensorRunBeforeReadyStartsNothing pins that no sensor run or
// adapter mounts before the Device setup target reports readiness.
func TestMountDeviceSensorRunBeforeReadyStartsNothing(t *testing.T) {
	statePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(statePath, "device"), 0o755); err != nil {
		t.Fatalf("create device state dir: %v", err)
	}

	run, err := mountDeviceSensorRun(
		context.Background(),
		logrus.WithField("test", t.Name()),
		statePath,
		nil,
	)
	if err != nil {
		t.Fatalf("mountDeviceSensorRun() error = %v", err)
	}
	if run != nil {
		run.release()
		t.Fatal("pre-readiness mount started a device sensor run")
	}
}

func TestComputeSensorCapabilityStateMapsLiveConnection(t *testing.T) {
	blocked := &s4wave_device.DeviceCapability{
		Detail: "Space denied sensor",
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
		},
	}
	cases := []struct {
		name          string
		status        *sensor.Status
		existing      *s4wave_device.DeviceCapability
		wantState     s4wave_device.DeviceCapabilityState
		wantDetailSub string
	}{
		{
			name:          "blocked grant stays blocked with detail",
			existing:      blocked,
			wantState:     s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED,
			wantDetailSub: "Space denied sensor",
		},
		{
			name:          "no live status waits for device session",
			wantState:     s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			wantDetailSub: "waiting for device session",
		},
		{
			name:          "connecting is available",
			status:        &sensor.Status{ConnectionState: s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTING},
			wantState:     s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			wantDetailSub: "connecting",
		},
		{
			name:          "connected is active with no detail",
			status:        &sensor.Status{ConnectionState: s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTED},
			wantState:     s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
			wantDetailSub: "",
		},
		{
			name: "degraded keeps the keepalive failure visible",
			status: &sensor.Status{
				ConnectionState: s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_DEGRADED,
				LastError:       "keepalive timed out",
			},
			wantState:     s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			wantDetailSub: "degraded: keepalive timed out",
		},
		{
			name: "offline reports the last connection error",
			status: &sensor.Status{
				ConnectionState: s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_OFFLINE,
				LastError:       "connection refused",
			},
			wantState:     s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			wantDetailSub: "offline: connection refused",
		},
	}
	for _, tc := range cases {
		gotState, gotDetail := computeSensorCapabilityState(tc.status, tc.existing)
		if gotState != tc.wantState {
			t.Fatalf("%s: state = %v, want %v", tc.name, gotState, tc.wantState)
		}
		if tc.wantDetailSub != "" && gotDetail != tc.wantDetailSub {
			t.Fatalf("%s: detail = %q, want %q", tc.name, gotDetail, tc.wantDetailSub)
		}
		if tc.name == "connected is active with no detail" && gotDetail != "" {
			t.Fatalf("%s: detail = %q, want empty", tc.name, gotDetail)
		}
	}
}

func TestComputeSensorCapabilityLinksTheSensorObject(t *testing.T) {
	endpoint := &device_policy.SensorEndpointPolicy{
		Id:          "radar",
		Enabled:     true,
		AdapterKind: s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME,
	}
	capability := computeSensorCapability(
		&device_policy.DevicePolicy{},
		endpoint,
		"devices/test-device",
		nil,
		nil,
	)
	if capability.GetId() != "sensor-radar" || capability.GetKind() != "sensor" {
		t.Fatalf("capability id/kind = %s/%s, want sensor-radar/sensor", capability.GetId(), capability.GetKind())
	}
	link := capability.GetLink()
	if link.GetTypeId() != s4wave_device.SensorTypeID {
		t.Fatalf("link type id = %q, want %q", link.GetTypeId(), s4wave_device.SensorTypeID)
	}
	if link.GetObjectKey() != sensor.ObjectKey("devices/test-device", "radar") {
		t.Fatalf("link object key = %q, want the derived sensor key", link.GetObjectKey())
	}
	if capability.GetLabel() != "radar sensor" {
		t.Fatalf("label = %q, want %q", capability.GetLabel(), "radar sensor")
	}
}
