package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/store"
	"github.com/aperturerobotics/hydra/store/mqueue"
)

// TestAll runs all tests.
func TestAll(t *testing.T, ktx store.Store) {
	TestMQueueE2E(t, ktx)
	TestObjectStore(t, ktx)
}

// TestObjectStore tests the object store.
func TestObjectStore(t *testing.T, ktx store.Store) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	obj, err := ktx.OpenObjectStore(ctx, "test-store")
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := obj.SetObject("test", []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err.Error())
	}

	ks, err := obj.ListKeys("t")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(ks) != 1 {
		t.FailNow()
	}
	if ks[0] != "test" {
		t.FailNow()
	}

	dat, found, err := obj.GetObject("test")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.FailNow()
	}
	if bytes.Compare(dat, []byte{1, 2, 3, 4}) != 0 {
		t.FailNow()
	}

	if err := obj.DeleteObject("test"); err != nil {
		t.Fatal(err.Error())
	}

	dat, found, err = obj.GetObject("test")
	if err != nil {
		t.Fatal(err.Error())
	}
	if found || len(dat) != 0 {
		t.FailNow()
	}

	if err := ktx.DelObjectStore(ctx, "test-store"); err != nil {
		t.Fatal(err.Error())
	}
}

// TestMQueueE2E tests a message queue end to end.
func TestMQueueE2E(t *testing.T, ktx store.Store) {
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
