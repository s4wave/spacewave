package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/bucket/store"
	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/store"
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

	obj, err := ktx.OpenObjectStore(ctx, "test-store-2")
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := kvtx_kvtest.TestAll(ctx, obj); err != nil {
		t.Fatal(err.Error())
	}

	if err := ktx.DelObjectStore(ctx, "test-store-2"); err != nil {
		t.Fatal(err.Error())
	}

	obj, err = ktx.OpenObjectStore(ctx, "test-store")
	if err != nil {
		t.Fatal(err.Error())
	}

	objTx, err := obj.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := objTx.Set([]byte("test"), []byte{1, 2, 3, 4}, 0); err != nil {
		t.Fatal(err.Error())
	}
	if err := objTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	objTx, err = obj.NewTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	var ks [][]byte
	err = objTx.ScanPrefix([]byte("t"), func(key, val []byte) error {
		k := make([]byte, len(key))
		copy(k, key)
		ks = append(ks, k)
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(ks) != 1 {
		t.FailNow()
	}
	if string(ks[0]) != "test" {
		t.FailNow()
	}
	objTx.Discard()

	objTx, err = obj.NewTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	dat, found, err := objTx.Get([]byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.FailNow()
	}
	if bytes.Compare(dat, []byte{1, 2, 3, 4}) != 0 {
		t.FailNow()
	}
	objTx.Discard()

	objTx, err = obj.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := objTx.Delete([]byte("test")); err != nil {
		t.Fatal(err.Error())
	}
	err = objTx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	objTx, err = obj.NewTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	dat, found, err = objTx.Get([]byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if found || len(dat) != 0 {
		t.FailNow()
	}
	objTx.Discard()
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
