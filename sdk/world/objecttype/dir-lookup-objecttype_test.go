package objecttype

import "testing"

func TestLookupObjectTypeEquivalenceIncludesEngineScope(t *testing.T) {
	first := NewLookupObjectTypeForEngine("test/type", "engine-a").(*lookupObjectType)
	if !first.IsEquivalent(NewLookupObjectTypeForEngine("test/type", "engine-a")) {
		t.Fatal("matching type and engine scope were not equivalent")
	}
	if first.IsEquivalent(NewLookupObjectTypeForEngine("test/type", "engine-b")) {
		t.Fatal("lookup directives from different engine scopes were equivalent")
	}
	if first.IsEquivalent(NewLookupObjectType("test/type")) {
		t.Fatal("scoped and unscoped lookup directives were equivalent")
	}
}
