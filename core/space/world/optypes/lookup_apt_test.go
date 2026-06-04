//go:build !tinygo && !goscript

package optypes

import (
	"context"
	"testing"

	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
)

func TestBuildSpaceLookupOpResolvesNativeAptOps(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), s4wave_apt.CreateAptRepositoryOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_apt.CreateAptRepositoryOp); !ok {
		t.Fatalf("expected CreateAptRepositoryOp, got %T", op)
	}
}
