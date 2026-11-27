//go:build !tinygo && !goscript

package objecttypes

import (
	"context"
	"testing"

	space_world "github.com/s4wave/spacewave/core/space/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestLookupObjectType covers the Device and SpaceSettings object types in the
// native registry. The GoScript registry has its own browser-tagged coverage.
func TestLookupObjectType(t *testing.T) {
	for _, row := range []struct{ typeID, want string }{
		{s4wave_device.DeviceTypeID, s4wave_device.DeviceTypeID},
		{space_world.SpaceSettingsTypeID, space_world.SpaceSettingsTypeID},
	} {
		got, err := LookupObjectType(context.Background(), row.typeID)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.GetObjectTypeID() != row.want {
			t.Fatalf("LookupObjectType(%q) = %v, want %q", row.typeID, got, row.want)
		}
	}
}

// TestBuiltInInventoryLookupParity resolves every listed type to itself.
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
