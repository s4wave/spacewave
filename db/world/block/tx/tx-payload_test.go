package world_block_tx_test

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestTxPayloadRefs checks that a replayed batch reports the payload of an
// operation recorded as an object operation and as a World operation, since
// replay keeps only the payloads it reports.
func TestTxPayloadRefs(t *testing.T) {
	// Build a write of one blob for each form.
	var blobs []*block.BlockRef
	var txs []*world_block_tx.Tx
	for i, form := range []string{"object", "world"} {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(form))
		if err != nil {
			t.Fatal(err.Error())
		}
		blob := block.NewBlockRef(h)
		blobs = append(blobs, blob)
		path := &unixfs_block.FSPath{Nodes: []string{form}}
		op := unixfs_world.NewFsWriteAtOp("fs", unixfs_world.FSType_FSType_FS_NODE, path, 0, blob, time.Unix(0, 0))

		// Record it as the WorldState records each form.
		var tx *world_block_tx.Tx
		if i == 0 {
			tx, err = world_block_tx.NewTxApplyObjectOp(op.GetOperationTypeId(), op, "fs", peer.ID(""))
		} else {
			tx, err = world_block_tx.NewTxApplyWorldOp(op, peer.ID(""))
		}
		if err != nil {
			t.Fatal(err.Error())
		}
		txs = append(txs, tx)
	}
	batch, err := world_block_tx.NewTxBatch(&world_block_tx.TxBatch{Txs: txs})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Report both blobs in order.
	refs, err := batch.PayloadRefs(t.Context(), unixfs_world.LookupFsOp)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(refs) != len(blobs) {
		t.Fatalf("expected %d payload roots, got %d", len(blobs), len(refs))
	}
	for i, ref := range refs {
		if !ref.EqualsRef(blobs[i]) {
			t.Fatalf("payload %d: expected %s, got %s", i, blobs[i].MarshalString(), ref.MarshalString())
		}
	}
}
