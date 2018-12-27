package kvtx_inmem

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/store/mqueue"
	"github.com/sirupsen/logrus"
)

// TestKVTxMQueue tests a key/value transaction message queue on top of inmem.
func TestKVTxMQueue(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	ktx := kvtx.NewKVTx(ctx, kvkey, kvtx_vlogger.NewVLogger(le, NewStore()))
	pair := bucket_store.BucketReconcilerPair{
		BucketID:     "test-bucket",
		ReconcilerID: "test-reconciler",
	}
	mq, err := ktx.GetReconcilerEventQueue(pair)
	if err != nil {
		t.Fatal(err.Error())
	}

	checkNoMsg := func() {
		msg, ok, err := mq.Peek()
		if err != nil {
			t.Fatal(err.Error())
		}
		if ok || msg != nil {
			t.Fatal("expected !ok when no messages")
		}
	}
	checkNoMsg()

	testData := "test"
	checkMsg := func(m mqueue.Message) {
		if bytes.Compare(m.GetData(), []byte(testData)) != 0 {
			t.Fatal("compared data, was different")
		}
	}

	pushedMsg, err := mq.Push([]byte(testData))
	if err != nil {
		t.Fatal(err.Error())
	}
	checkMsg(pushedMsg)

	peekedMsg, ok, err := mq.Peek()
	if !ok || peekedMsg == nil {
		t.Fatal("expected peek() to be ok after push()")
	}
	checkMsg(peekedMsg)

	if err := mq.Ack(peekedMsg.GetId()); err != nil {
		t.Fatal(err.Error())
	}
	checkNoMsg()

	pushedMsg, err = mq.Push([]byte(testData))
	if err != nil {
		t.Fatal(err.Error())
	}
	checkMsg(pushedMsg)

	if err := ktx.DeleteReconcilerEventQueue(pair); err != nil {
		t.Fatal(err.Error())
	}
	checkNoMsg()
}
