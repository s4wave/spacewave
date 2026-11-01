//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestDeviceLabel sets, replaces and removes persisted selectors through the CLI writer.
func TestDeviceLabel(t *testing.T) {
	// Create a real Device in the in-memory World.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	const key = "devices/forge"
	err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, key, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_device.Device{PeerId: "peer", Label: "forge", Platform: &s4wave_device.DevicePlatform{Os: "linux", Arch: "arm64"}}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, ws, key, s4wave_device.DeviceTypeID)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Use the production label writer and verify both stored and derived values.
	if err := labelDeviceInWorld(ctx, tb.Engine, key, []string{"gpu=true", "remove=yes", "kubernetes.io/arch=amd64"}); err != nil {
		t.Fatal(err)
	}
	if err := labelDeviceInWorld(ctx, tb.Engine, key, []string{"remove-", "gpu=ready"}); err != nil {
		t.Fatal(err)
	}
	read := func() *s4wave_device.Device {
		// Read the Device from one committed World snapshot.
		var device *s4wave_device.Device
		err := world.ExecTransaction(ctx, tb.Engine, false, func(ctx context.Context, ws world.WorldState) error {
			var err error
			device, err = world.LookupObjectBody[s4wave_device.Device](ctx, ws, key)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return device
	}
	device := read()
	labels := device.EffectiveLabels(key)
	if len(device.Labels) != 2 || labels["gpu"] != "ready" || labels["kubernetes.io/hostname"] != "forge" || labels["kubernetes.io/os"] != "linux" || labels["kubernetes.io/arch"] != "amd64" {
		t.Fatalf("Device labels = %#v, effective = %#v", device.Labels, labels)
	}

	// Read the human and machine output through the production Device show writers.
	var shown bytes.Buffer
	writeDeviceShow(&shown, key, device, forge_runtime.DeviceAvailability{})
	for _, value := range []string{"gpu", "ready", "kubernetes.io/hostname", "forge", "kubernetes.io/os", "linux", "amd64"} {
		if !strings.Contains(shown.String(), value) {
			t.Fatalf("Device show omitted %q: %s", value, shown.String())
		}
	}

	// JSON includes the same effective selectors beside the stored label map.
	data, err := marshalDeviceShow(key, device, forge_runtime.DeviceAvailability{})
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	doc, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(doc.GetStringBytes("effectiveLabels", "kubernetes.io/arch")) != "amd64" {
		t.Fatalf("Device JSON omitted effective labels: %s", data)
	}

	// A later daemon projection keeps the independently written selectors.
	projected := &s4wave_device.Device{}
	mergeDeviceObjectState(projected, device)
	if projected.GetLabels()["gpu"] != "ready" {
		t.Fatal("daemon projection lost stored selectors")
	}
	if err := labelDeviceInWorld(ctx, tb.Engine, key, []string{"gpu=changed", "invalid"}); err == nil {
		t.Fatal("invalid edit accepted")
	}
	device = read()
	if device.GetLabels()["gpu"] != "ready" {
		t.Fatalf("invalid batch changed labels: %v, %v", device, err)
	}

	// Exercise the actual registered command's help without needing a daemon.
	if err := runDeviceCLI(t, "device", "label", "--help"); err != nil {
		t.Fatal(err)
	}
}
