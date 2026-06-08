package bldr_dist

import (
	"testing"

	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/net/hash"
)

func TestDistBucketConfigDoesNotWriteBackStaticFallbackBlocks(t *testing.T) {
	conf, err := NewDistBucketConfig("spacewave")
	if err != nil {
		t.Fatalf("NewDistBucketConfig failed: %v", err)
	}

	lookup := &lookup_concurrent.Config{}
	if err := lookup.UnmarshalVT(conf.GetLookup().GetController().GetConfig()); err != nil {
		t.Fatalf("unmarshal lookup config failed: %v", err)
	}
	if lookup.GetFallbackBlockStoreId() != StaticBlockStoreID {
		t.Fatalf("fallback block store = %q, want %q", lookup.GetFallbackBlockStoreId(), StaticBlockStoreID)
	}
	if lookup.GetWritebackBehavior() != lookup_concurrent.WritebackBehavior_WritebackBehavior_NONE {
		t.Fatalf("writeback behavior = %v, want none", lookup.GetWritebackBehavior())
	}
	if lookup.GetPutBlockBehavior() != lookup_concurrent.PutBlockBehavior_PutBlockBehavior_ALL {
		t.Fatalf("put block behavior = %v, want all", lookup.GetPutBlockBehavior())
	}
	if got := conf.GetPutOpts().GetHashType(); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("put opts hash type = %v, want SHA256", got)
	}
}
