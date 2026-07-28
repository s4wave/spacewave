//go:build !tinygo

package objecttypes

import (
	"context"
	"testing"

	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

func TestKvObjectTypeMetadataCoverage(t *testing.T) {
	ctx := context.Background()
	for _, row := range []struct {
		typeID   string
		opTypeID string
	}{
		{s4wave_kv_world.KvStoreTypeID, s4wave_kv_world.KvSetRootOpId},
	} {
		objType, err := LookupObjectType(ctx, row.typeID)
		if err != nil {
			t.Fatalf("LookupObjectType(%s): %v", row.typeID, err)
		}
		if objType == nil {
			t.Fatalf("LookupObjectType(%s) returned nil", row.typeID)
		}
		if got := objType.GetObjectTypeID(); got != row.typeID {
			t.Fatalf("LookupObjectType(%s) id = %s", row.typeID, got)
		}
		op, err := space_world_optypes.LookupWorldOp(ctx, row.opTypeID)
		if err != nil {
			t.Fatalf("LookupWorldOp(%s): %v", row.opTypeID, err)
		}
		if _, ok := op.(*s4wave_kv_world.KvSetRootOp); !ok {
			t.Fatalf("LookupWorldOp(%s) = %T, want *KvSetRootOp", row.opTypeID, op)
		}
	}
}
