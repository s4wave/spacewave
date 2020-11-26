package store_test

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/bucket/store"
	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/store"
	"github.com/pkg/errors"
)

// TestAll runs all tests.
func TestAll(ktx store.Store) error {
	if err := TestMQueueE2E(ktx); err != nil {
		return err
	}
	if err := TestObjectStore(ktx); err != nil {
		return err
	}
	return nil
}

// TestObjectStore tests the object store.
func TestObjectStore(ktx store.Store) error {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	obj, err := ktx.OpenObjectStore(ctx, "test-store-2")
	if err != nil {
		return err
	}

	if err := kvtx_kvtest.TestAll(ctx, obj); err != nil {
		return err
	}

	if err := ktx.DelObjectStore(ctx, "test-store-2"); err != nil {
		return err
	}

	obj, err = ktx.OpenObjectStore(ctx, "test-store")
	if err != nil {
		return err
	}

	objTx, err := obj.NewTransaction(true)
	if err != nil {
		return err
	}
	if err := objTx.Set([]byte("test"), []byte{1, 2, 3, 4}, 0); err != nil {
		return err
	}
	if err := objTx.Commit(ctx); err != nil {
		return err
	}

	objTx, err = obj.NewTransaction(false)
	if err != nil {
		return err
	}
	var ks [][]byte
	err = objTx.ScanPrefix([]byte("t"), func(key, val []byte) error {
		k := make([]byte, len(key))
		copy(k, key)
		ks = append(ks, k)
		return nil
	})
	if err != nil {
		return err
	}
	if len(ks) != 1 {
		return errors.Errorf("expected slice len 1: %v", ks)
	}
	if string(ks[0]) != "test" {
		return errors.Errorf("expected single entry 'test' %v", ks[0])
	}
	objTx.Discard()

	objTx, err = obj.NewTransaction(false)
	if err != nil {
		return err
	}
	dat, found, err := objTx.Get([]byte("test"))
	if err != nil {
		return err
	}
	if !found {
		return errors.New("expected to find key test")
	}
	if bytes.Compare(dat, []byte{1, 2, 3, 4}) != 0 {
		return errors.New("incorrect value in data")
	}
	objTx.Discard()

	objTx, err = obj.NewTransaction(true)
	if err != nil {
		return err
	}
	if err := objTx.Delete([]byte("test")); err != nil {
		return err
	}
	err = objTx.Commit(ctx)
	if err != nil {
		return err
	}

	objTx, err = obj.NewTransaction(false)
	if err != nil {
		return err
	}
	dat, found, err = objTx.Get([]byte("test"))
	if err != nil {
		return err
	}
	if found || len(dat) != 0 {
		return errors.New("expected not found")
	}
	objTx.Discard()
	if err := ktx.DelObjectStore(ctx, "test-store"); err != nil {
		return err
	}

	return nil
}

// TestMQueueE2E tests a message queue end to end.
func TestMQueueE2E(ktx store.Store) error {
	pair := bucket_store.BucketReconcilerPair{
		BucketID:     "test-bucket",
		ReconcilerID: "test-reconciler",
	}
	mq, err := ktx.GetReconcilerEventQueue(pair)
	if err != nil {
		return err
	}

	checkNoMsg := func() error {
		msg, ok, err := mq.Peek()
		if err != nil {
			return err
		}
		if ok || msg != nil {
			return errors.New("expected !ok when no messages")
		}
		return nil
	}
	if err := checkNoMsg(); err != nil {
		return err
	}

	testData := "test"
	checkMsg := func(m mqueue.Message) error {
		if bytes.Compare(m.GetData(), []byte(testData)) != 0 {
			return errors.New("compared data, was different")
		}
		return nil
	}

	// break kvtx/test/test.go:42
	pushedMsg, err := mq.Push([]byte(testData))
	if err != nil {
		return err
	}
	if err := checkMsg(pushedMsg); err != nil {
		return err
	}

	pairs, err := ktx.ListFilledReconcilerEventQueues()
	if err != nil {
		return err
	}
	if len(pairs) != 1 {
		return errors.New("expected 1 pair")
	}

	peekedMsg, ok, err := mq.Peek()
	if !ok || peekedMsg == nil {
		return errors.New("expected peek() to be ok after push()")
	}
	checkMsg(peekedMsg)

	if err := mq.Ack(peekedMsg.GetId()); err != nil {
		return err
	}
	if err := checkNoMsg(); err != nil {
		return err
	}

	pushedMsg, err = mq.Push([]byte(testData))
	if err != nil {
		return err
	}
	if err := checkMsg(pushedMsg); err != nil {
		return err
	}

	if err := ktx.DeleteReconcilerEventQueue(pair); err != nil {
		return err
	}
	if err := checkNoMsg(); err != nil {
		return err
	}
	return nil
}
