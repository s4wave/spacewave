package filters

import (
	"testing"

	quad "github.com/s4wave/spacewave/db/block/quad"
)

// TestKeyFiltersBuilder runs basic tests on the key filters builder.
func TestKeyFiltersBuilder(t *testing.T) {
	kfb := NewKeyFiltersBuilder(1024)
	kfb.ApplyObjectKey("hello/world")
	kfb.ApplyObjectKey("hello")
	kf := kfb.BuildKeyFilters()
	if kf.GetKeyPrefix() != "hello" {
		t.FailNow()
	}

	kfb = NewKeyFiltersBuilder(1024)
	kfb.ApplyQuad(&quad.Quad{
		Subject:   "this-was",
		Predicate: "is",
		Obj:       "that-was",
	})
	kfb.ApplyQuad(&quad.Quad{
		Subject:   "this-there",
		Predicate: "are",
		Obj:       "that-them",
	})
	kf = kfb.BuildKeyFilters()
	if kf.GetQuadPrefix().GetSubject() != "this-" {
		t.FailNow()
	}
	if kf.GetQuadPrefix().GetObj() != "that-" {
		t.FailNow()
	}
}
