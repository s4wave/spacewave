//go:build !tinygo && !goscript

package objecttypes

import (
	"context"
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestLookupDeviceObjectType covers the Device object type in the native
// registry; the GoScript registry has its own browser-tagged coverage.
func TestLookupDeviceObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_device.DeviceTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Device ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_device.DeviceTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_device.DeviceTypeID)
	}
}
