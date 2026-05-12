package world

import (
	"context"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/refs"
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

func TestCachedCayleyHandleCachesReadOperations(t *testing.T) {
	ctx := context.Background()
	fake := &cachedCayleyHandleTestStore{
		valueRef: testGraphRef("value-ref"),
		quadRef:  testGraphRef("quad-ref"),
	}
	handle := NewCachedCayleyHandle(fake)
	value := quad.IRI("<rel>")

	for range 2 {
		ref, err := handle.ValueOf(ctx, value)
		if err != nil {
			t.Fatal(err.Error())
		}
		if ref.Key() != fake.valueRef.Key() {
			t.Fatalf("value ref = %v, want %v", ref.Key(), fake.valueRef.Key())
		}
		size, err := handle.QuadIteratorSize(ctx, quad.Predicate, ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if size.Value != 7 {
			t.Fatalf("size = %d, want 7", size.Value)
		}
		q, err := handle.Quad(ctx, fake.quadRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		if q.Predicate.String() != value.String() {
			t.Fatalf("quad predicate = %s, want %s", q.Predicate.String(), value.String())
		}
		dirRef, err := handle.QuadDirection(ctx, fake.quadRef, quad.Predicate)
		if err != nil {
			t.Fatal(err.Error())
		}
		if dirRef.Key() != fake.valueRef.Key() {
			t.Fatalf("direction ref = %v, want %v", dirRef.Key(), fake.valueRef.Key())
		}
	}

	if fake.valueOfCalls != 1 {
		t.Fatalf("ValueOf calls = %d, want 1", fake.valueOfCalls)
	}
	if fake.sizeCalls != 1 {
		t.Fatalf("QuadIteratorSize calls = %d, want 1", fake.sizeCalls)
	}
	if fake.nameOfCalls != 1 {
		t.Fatalf("NameOf calls = %d, want 1", fake.nameOfCalls)
	}
	if fake.quadCalls != 0 {
		t.Fatalf("Quad calls = %d, want 0", fake.quadCalls)
	}
	if fake.directionCalls != 4 {
		t.Fatalf("QuadDirection calls = %d, want 4", fake.directionCalls)
	}
}

func TestReadOperationCayleyHandleSkipsQuadRefCaches(t *testing.T) {
	ctx := context.Background()
	fake := &cachedCayleyHandleTestStore{
		valueRef: testGraphRef("value-ref"),
		quadRef:  testGraphRef("quad-ref"),
	}
	handle := NewReadOperationCayleyHandle(fake)

	for range 2 {
		q, err := handle.Quad(ctx, fake.quadRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		if q.Subject.String() != "<<rel>>" {
			t.Fatalf("quad subject = %s, want <<rel>>", q.Subject.String())
		}
	}
	if fake.directionCalls != 8 {
		t.Fatalf("QuadDirection calls = %d, want 8", fake.directionCalls)
	}
	if fake.nameOfCalls != 1 {
		t.Fatalf("NameOf calls = %d, want 1", fake.nameOfCalls)
	}
}

type cachedCayleyHandleTestStore struct {
	CayleyHandle

	valueRef       graph.Ref
	quadRef        graph.Ref
	valueOfCalls   int
	nameOfCalls    int
	sizeCalls      int
	quadCalls      int
	directionCalls int
}

func (s *cachedCayleyHandleTestStore) ValueOf(ctx context.Context, value quad.Value) (graph.Ref, error) {
	s.valueOfCalls++
	return s.valueRef, nil
}

func (s *cachedCayleyHandleTestStore) NameOf(ctx context.Context, ref graph.Ref) (quad.Value, error) {
	s.nameOfCalls++
	return quad.IRI("<rel>"), nil
}

func (s *cachedCayleyHandleTestStore) QuadIteratorSize(ctx context.Context, dir quad.Direction, ref graph.Ref) (refs.Size, error) {
	s.sizeCalls++
	return refs.Size{Value: 7, Exact: true}, nil
}

func (s *cachedCayleyHandleTestStore) Quad(ctx context.Context, ref graph.Ref) (quad.Quad, error) {
	s.quadCalls++
	return quad.Quad{
		Subject:   quad.IRI("<subject>"),
		Predicate: quad.IRI("<rel>"),
		Object:    quad.IRI("<object>"),
	}, nil
}

func (s *cachedCayleyHandleTestStore) QuadDirection(ctx context.Context, ref graph.Ref, dir quad.Direction) (graph.Ref, error) {
	s.directionCalls++
	return s.valueRef, nil
}

type testGraphRef string

func (r testGraphRef) Key() any {
	return string(r)
}
