package optypes

import (
	"context"
	"testing"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
)

func TestBuildSpaceLookupOpResolvesBuiltInWithoutBus(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), space_world_ops.InitUnixFSOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*space_world_ops.InitUnixFSOp); !ok {
		t.Fatalf("expected InitUnixFSOp, got %T", op)
	}
}

func TestBuildSpaceLookupOpReturnsNilForUnknownWithoutBus(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), "space/world/custom-op")
	if err != nil {
		t.Fatal(err)
	}
	if op != nil {
		t.Fatalf("expected nil op, got %T", op)
	}
}
