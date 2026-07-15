//go:build tinygo

package objecttypes

import "testing"

func TestCompiledInventoryLookupParityUnderTinyGo(t *testing.T) {
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
