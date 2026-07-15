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

func TestBuiltInInventoryLookupParity(t *testing.T) {
	ctx := context.Background()
	for _, typeID := range BuiltInObjectTypeIDs() {
		objType, err := LookupObjectType(ctx, typeID)
		if err != nil {
			t.Fatalf("LookupObjectType(%q): %v", typeID, err)
		}
		if objType == nil {
			t.Fatalf("LookupObjectType(%q) returned nil", typeID)
		}
		if got := objType.GetObjectTypeID(); got != typeID {
			t.Fatalf("LookupObjectType(%q) returned %q", typeID, got)
		}
	}
}
