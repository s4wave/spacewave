package world_block_tx_test

import (
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blocktest"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestTxPayloadRefs checks that a replayed batch reports the payload of an
// operation recorded as an object operation and as a World operation, and the
// root a direct object set writes, since replay keeps only the payloads it
// reports.
func TestTxPayloadRefs(t *testing.T) {
	// Build a write of one blob for each form.
	var blobs []*block.BlockRef
	var txs []*world_block_tx.Tx
	for i, form := range []string{"object", "world", "set"} {
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
		switch i {
		case 0:
			tx, err = world_block_tx.NewTxApplyObjectOp(op.TypeID(), op, "fs", peer.ID(""))
		case 1:
			tx, err = world_block_tx.NewTxApplyWorldOp(op, peer.ID(""))
		default:
			tx, err = world_block_tx.NewTxObjectSet("fs", &bucket.ObjectRef{RootRef: blob})
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
	refs, err := batch.PayloadRefs(t.Context(), world.NewSchema(unixfs_world.UnixfsTypes).LookupOp)
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

// txPayloadRefsGolden records, for each transaction type, a fixture and the
// roots PayloadRefs returns for it under PayloadRefsVersion. Each fixture sets
// every block reference the transaction carries to a distinct named root, and
// refs lists the names of the roots PayloadRefs returns, in order. An applied
// operation's own payload is a FsWriteAtOp referencing its blob, whose version
// the operation golden test covers.
//
// A saved replay cursor reuses a transaction's roots only under the same
// version, so when what PayloadRefs returns changes, bump PayloadRefsVersion
// and record the new refs here together.
var txPayloadRefsGolden = map[world_block_tx.TxType]struct {
	tx   func(t *testing.T, ref func(name string) *block.BlockRef) *world_block_tx.Tx
	refs []string
}{
	world_block_tx.TxType_TxType_INVALID: {
		tx: func(*testing.T, func(string) *block.BlockRef) *world_block_tx.Tx {
			return &world_block_tx.Tx{}
		},
	},
	world_block_tx.TxType_TxType_APPLY_WORLD_OP: {
		tx: func(t *testing.T, ref func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxApplyWorldOp(payloadOp("op-payload", ref), peer.ID(""))
			if err != nil {
				t.Fatal(err)
			}
			tx.TxApplyWorldOp.SetRoots = []*block.BlockRef{ref("set-root")}
			return tx
		},
		refs: []string{"set-root", "op-payload"},
	},
	world_block_tx.TxType_TxType_APPLY_OBJECT_OP: {
		tx: func(t *testing.T, ref func(string) *block.BlockRef) *world_block_tx.Tx {
			op := payloadOp("op-payload", ref)
			tx, err := world_block_tx.NewTxApplyObjectOp(op.TypeID(), op, "fs", peer.ID(""))
			if err != nil {
				t.Fatal(err)
			}
			tx.TxApplyObjectOp.SetRoots = []*block.BlockRef{ref("set-root")}
			return tx
		},
		refs: []string{"set-root", "op-payload"},
	},
	world_block_tx.TxType_TxType_CREATE_OBJECT: {
		tx: func(t *testing.T, ref func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxCreateObject("obj", objectRef("root", ref))
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
		refs: []string{"root"},
	},
	world_block_tx.TxType_TxType_OBJECT_SET: {
		tx: func(t *testing.T, ref func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxObjectSet("obj", objectRef("root", ref))
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
		refs: []string{"root"},
	},
	world_block_tx.TxType_TxType_OBJECT_INC_REV: {
		tx: func(t *testing.T, _ func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxObjectIncRev("obj")
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
	},
	world_block_tx.TxType_TxType_DELETE_OBJECT: {
		tx: func(t *testing.T, _ func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxDeleteObject("obj")
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
	},
	world_block_tx.TxType_TxType_RENAME_OBJECT: {
		tx: func(t *testing.T, _ func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxRenameObject("old", "new")
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
	},
	world_block_tx.TxType_TxType_SET_GRAPH_QUAD: {
		tx: func(t *testing.T, _ func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxSetGraphQuad(&quad.Quad{Subject: "<s>", Predicate: "<p>", Obj: "<o>"})
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
	},
	world_block_tx.TxType_TxType_DELETE_GRAPH_QUAD: {
		tx: func(t *testing.T, _ func(string) *block.BlockRef) *world_block_tx.Tx {
			tx, err := world_block_tx.NewTxDeleteGraphQuad(&quad.Quad{Subject: "<s>", Predicate: "<p>", Obj: "<o>"})
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
	},
	world_block_tx.TxType_TxType_BATCH: {
		tx: func(t *testing.T, ref func(string) *block.BlockRef) *world_block_tx.Tx {
			set, err := world_block_tx.NewTxObjectSet("obj", objectRef("batch-root", ref))
			if err != nil {
				t.Fatal(err)
			}
			apply, err := world_block_tx.NewTxApplyWorldOp(payloadOp("batch-op-payload", ref), peer.ID(""))
			if err != nil {
				t.Fatal(err)
			}
			apply.TxApplyWorldOp.SetRoots = []*block.BlockRef{ref("batch-set-root")}
			tx, err := world_block_tx.NewTxBatch(&world_block_tx.TxBatch{Txs: []*world_block_tx.Tx{set, apply}})
			if err != nil {
				t.Fatal(err)
			}
			return tx
		},
		refs: []string{"batch-root", "batch-set-root", "batch-op-payload"},
	},
}

// txPayloadRefsVersion is the PayloadRefsVersion txPayloadRefsGolden records.
const txPayloadRefsVersion uint32 = 1

// payloadOp returns a payload operation whose payload is the root named name.
func payloadOp(name string, ref func(string) *block.BlockRef) world.Operation {
	path := &unixfs_block.FSPath{Nodes: []string{"file"}}
	return unixfs_world.NewFsWriteAtOp("fs", unixfs_world.FSType_FSType_FS_NODE, path, 0, ref(name), time.Unix(0, 0))
}

// objectRef returns an object reference rooted at the root named name, with
// every other reference it carries set.
func objectRef(name string, ref func(string) *block.BlockRef) *bucket.ObjectRef {
	return &bucket.ObjectRef{RootRef: ref(name), TransformConfRef: ref(name + "-transform-conf")}
}

// unsetTxRefs returns the block references the transaction's body can hold
// that its fixture leaves unset, walking each transaction of a batch.
func unsetTxRefs(t *testing.T, tx *world_block_tx.Tx) []string {
	switch tx.GetTxType() {
	case world_block_tx.TxType_TxType_INVALID:
		return nil
	case world_block_tx.TxType_TxType_BATCH:
		var unset []string
		for _, sub := range tx.GetTxBatch().GetTxs() {
			unset = append(unset, unsetTxRefs(t, sub)...)
		}
		return unset
	}
	body, err := tx.LocateTx()
	if err != nil {
		t.Fatal(err)
	}
	return blocktest.UnsetRefs(body)
}

// TestTxPayloadRefsGolden checks that every transaction type has a golden
// fixture that sets every block reference it carries, and that what
// PayloadRefs returns changes only together with PayloadRefsVersion.
func TestTxPayloadRefsGolden(t *testing.T) {
	if world_block_tx.PayloadRefsVersion == 0 {
		t.Fatal("PayloadRefsVersion is zero")
	}
	lookupOp := world.NewSchema(unixfs_world.UnixfsTypes).LookupOp
	for value, typeName := range world_block_tx.TxType_name {
		txType := world_block_tx.TxType(value)
		golden, ok := txPayloadRefsGolden[txType]
		if !ok {
			t.Errorf("%s: transaction type has no golden fixture in txPayloadRefsGolden", typeName)
			continue
		}

		names := make(map[string]string)
		ref := func(name string) *block.BlockRef {
			r, err := block.BuildBlockRef([]byte(typeName+"/"+name), nil)
			if err != nil {
				t.Fatal(err)
			}
			names[r.MarshalString()] = name
			return r
		}
		tx := golden.tx(t, ref)
		if got := tx.GetTxType(); got != txType {
			t.Errorf("%s: fixture builds a %s transaction", typeName, got)
			continue
		}
		if unset := unsetTxRefs(t, tx); len(unset) != 0 {
			t.Errorf("%s: fixture leaves block references unset: %v", typeName, unset)
		}

		got, err := tx.PayloadRefs(t.Context(), lookupOp)
		if err != nil {
			t.Fatalf("%s: %v", typeName, err)
		}
		var refs []string
		for _, r := range got {
			name, ok := names[r.MarshalString()]
			if !ok {
				name = "unknown " + r.MarshalString()
			}
			refs = append(refs, name)
		}

		switch {
		case world_block_tx.PayloadRefsVersion != txPayloadRefsVersion:
			t.Errorf("%s: PayloadRefsVersion is %d, golden records %d with refs %v; record the refs it names now, %v, under the new version", typeName, world_block_tx.PayloadRefsVersion, txPayloadRefsVersion, golden.refs, refs)
		case !slices.Equal(refs, golden.refs):
			t.Errorf("%s: PayloadRefs names %v, golden records %v under the same version %d; bump PayloadRefsVersion and record the new refs", typeName, refs, golden.refs, txPayloadRefsVersion)
		}
	}
	for txType := range txPayloadRefsGolden {
		if _, ok := world_block_tx.TxType_name[int32(txType)]; !ok {
			t.Errorf("%d: golden fixture for a transaction type the enum does not name", txType)
		}
	}
}
