//go:build goscript

package objecttypes

import (
	"context"
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestLookupOmitsNativeOnlyObjectType pins that the GoScript browser subset
// omits the native-only Device object type so it does not drag native-only
// backends into the browser closure. ComputersDashboard stays resolvable
// (covered in device_test.go, which runs under this tag too).
func TestLookupOmitsNativeOnlyObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_device.DeviceTypeID)
	if err != nil {
		t.Fatalf("LookupObjectType(Device): %v", err)
	}
	if got != nil {
		t.Fatalf("browser subset resolved native-only Device type %q", got.GetObjectTypeID())
	}
}
