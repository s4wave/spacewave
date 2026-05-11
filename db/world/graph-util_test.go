package world

import (
	"testing"

	"github.com/aperturerobotics/cayley/quad"
)

func TestQuadFilterIteratorDirectionsPreferEndpointIndexes(t *testing.T) {
	filter := quad.Quad{
		Predicate: quad.IRI("<rel>"),
		Object:    quad.IRI("<target>"),
	}

	dirs := quadFilterIteratorDirections(filter)
	if len(dirs) != 2 {
		t.Fatalf("direction count: got %d want 2", len(dirs))
	}
	if dirs[0] != quad.Object {
		t.Fatalf("first direction: got %s want %s", dirs[0], quad.Object)
	}
	if dirs[1] != quad.Predicate {
		t.Fatalf("second direction: got %s want %s", dirs[1], quad.Predicate)
	}
	if quadFilterDirectionPriority(quad.Object) >= quadFilterDirectionPriority(quad.Predicate) {
		t.Fatal("object direction should outrank predicate direction")
	}
}
