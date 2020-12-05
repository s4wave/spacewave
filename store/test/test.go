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
