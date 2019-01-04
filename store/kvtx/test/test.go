package kvtx_test

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/store/mqueue"
)

// TestMQueueE2E tests a message queue end to end.
func TestMQueueE2E(t *testing.T, ktx *kvtx.KVTx) {
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

	// break kvtx/test/test.go:42
	pushedMsg, err := mq.Push([]byte(testData))
	if err != nil {
		t.Fatal(err.Error())
	}
	checkMsg(pushedMsg)

	pairs, err := ktx.ListFilledReconcilerEventQueues()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(pairs) != 1 {
		t.Fail()
	}

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
