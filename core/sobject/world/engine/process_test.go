//go:build !goscript

package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestProcessApplyTxOpRejectsUninitializedWorld(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	tx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		t.Fatalf("build tx: %v", err)
	}
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}

	_, res, err := (&Controller{}).processOp(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		nil,
		opData,
		"test-op",
		pid,
		1,
		0,
		&InnerState{},
	)
	if err != nil {
		t.Fatalf("process op returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected rejection result")
	}
	if res.GetSuccess() {
		t.Fatal("expected apply tx op to be rejected")
	}
	details := res.GetErrorDetails()
	if details.GetErrorMsg() != "world is not initialized" {
		t.Fatalf("expected world-not-initialized rejection, got %q", details.GetErrorMsg())
	}
}

func TestSOWorldOpSpeculativeLocalQueueSafeSkipsGCSweep(t *testing.T) {
	gcTx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		t.Fatal(err)
	}
	gcOp := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: gcTx},
		},
	}
	if gcOp.speculativeLocalQueueSafe() {
		t.Fatal("GC sweep should wait for authoritative processing")
	}

	createTx, err := world_block_tx.NewTxCreateObject("obj", &bucket.ObjectRef{})
	if err != nil {
		t.Fatal(err)
	}
	createOp := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: createTx},
		},
	}
	if !createOp.speculativeLocalQueueSafe() {
		t.Fatal("regular world transactions should still be speculative")
	}
}
