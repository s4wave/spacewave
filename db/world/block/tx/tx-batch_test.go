package world_block_tx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
)

// TestTxBatchClearBucketID checks that a replicated batch names object roots
// in the writer's bucket without its bucket ID, and keeps foreign buckets.
func TestTxBatchClearBucketID(t *testing.T) {
	// Build roots in the writer's bucket and in a foreign bucket.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("device"))
	if err != nil {
		t.Fatal(err.Error())
	}
	rootRef := block.NewBlockRef(h)
	local := &bucket.ObjectRef{RootRef: rootRef, BucketId: "writer"}
	foreign := &bucket.ObjectRef{RootRef: rootRef, BucketId: "other"}

	// Record a create and a set at the top level and in a nested batch.
	create, _ := NewTxCreateObject("a", local)
	set, _ := NewTxObjectSet("b", foreign)
	nested, _ := NewTxObjectSet("c", local)
	batch := &TxBatch{Txs: []*Tx{create, set, {TxType: TxType_TxType_BATCH, TxBatch: &TxBatch{Txs: []*Tx{nested}}}}}
	batch.ClearBucketID("writer")

	// Only roots in the writer's bucket lose their bucket ID.
	if got := create.GetTxCreateObject().GetRootRef().GetBucketId(); got != "" {
		t.Fatalf("create bucket = %q, want empty", got)
	}
	if got := set.GetTxObjectSet().GetRootRef().GetBucketId(); got != "other" {
		t.Fatalf("foreign set bucket = %q, want other", got)
	}
	if got := nested.GetTxObjectSet().GetRootRef().GetBucketId(); got != "" {
		t.Fatalf("nested set bucket = %q, want empty", got)
	}

	// The caller's ref is unchanged.
	if local.GetBucketId() != "writer" {
		t.Fatalf("caller ref bucket = %q, want writer", local.GetBucketId())
	}
}
