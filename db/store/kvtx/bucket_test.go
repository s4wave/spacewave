package store_kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestApplyBucketConfigRetriesInvalidSnapshot preserves bucket revisions after a commit conflict.
func TestApplyBucketConfigRetriesInvalidSnapshot(t *testing.T) {
	// Prepare a bucket store that invalidates the first commit attempt.
	ctx := t.Context()
	store := kvtx_kvtest.NewFaultStore(store_kvtx_inmem.NewStore(), kvtx_kvtest.FaultBeforeCommit)
	k := &KVTx{kvkey: store_kvkey.NewDefaultKVKey(), store: store}
	conf := &bucket.Config{Id: "space", Rev: 2}

	// Apply the bucket configuration across the invalidated snapshot.
	updated, previous, current, err := k.ApplyBucketConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}

	// Require the retried update to create the bucket and release both attempts.
	if !updated || previous != nil || !current.EqualVT(conf) {
		t.Fatalf("create returned updated=%v previous=%v current=%v", updated, previous, current)
	}
	if store.Opened() != 2 || store.Discarded() != 2 {
		t.Fatalf("transaction attempts: opened=%d discarded=%d", store.Opened(), store.Discarded())
	}

	// Read back the bucket configuration committed by the retry.
	persisted, err := k.GetBucketConfig(ctx, conf.Id)
	if err != nil || !persisted.EqualVT(conf) {
		t.Fatalf("persisted config=%v error=%v", persisted, err)
	}

	// An older revision cannot replace the configuration committed by the retry.
	updated, previous, current, err = k.ApplyBucketConfig(ctx, &bucket.Config{Id: conf.Id, Rev: 1})
	if err != nil || updated || !previous.EqualVT(conf) || !current.EqualVT(conf) {
		t.Fatalf("older revision returned updated=%v previous=%v current=%v error=%v", updated, previous, current, err)
	}
}
