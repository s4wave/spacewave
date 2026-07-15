//go:build goscript

package objecttypes

import (
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func TestLookupDeviceObjectTypeUnderGoScript(t *testing.T) {
	requireObjectType(t, s4wave_device.DeviceTypeID)
}

func TestLookupSqlObjectTypesExcludedFromCoreUnderGoScript(t *testing.T) {
	for _, typeID := range []string{
		"sql/db",
		"sql/query",
		"sql/query-result",
		"sql/schema",
		"sql/table-view",
		"sql/workbench",
	} {
		got, err := LookupObjectType(t.Context(), typeID)
		if err != nil {
			t.Fatalf("LookupObjectType(%s): %v", typeID, err)
		}
		if got != nil {
			t.Fatalf("LookupObjectType(%s) = %T, want nil", typeID, got)
		}
	}
}

func TestCompiledInventoryLookupParityUnderGoScript(t *testing.T) {
	for _, typeID := range BuiltInObjectTypeIDs() {
		got, err := LookupObjectType(t.Context(), typeID)
		if err != nil {
			t.Fatalf("LookupObjectType(%s): %v", typeID, err)
		}
		if got == nil || got.GetObjectTypeID() != typeID {
			t.Fatalf("LookupObjectType(%s) = %#v", typeID, got)
		}
	}
}
