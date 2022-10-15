package store_test

import (
	"bytes"
	"context"
	"time"

	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
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

	if err := ktx.RmObjectStore(ctx, "test-store-2"); err != nil {
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

	ctx := context.Background()
	checkNoMsg := func() error {
		msg, ok, err := mq.Peek()
		if err != nil {
			return err
		}
		if ok || msg != nil {
			return errors.New("expected !ok when no messages")
		}
		dctx, dctxCancel := context.WithTimeout(ctx, time.Millisecond*10)
		_, err = mq.Wait(dctx, false)
		dctxCancel()
		if err != context.DeadlineExceeded {
			return errors.Errorf("expected deadline exceeded but got %v", err)
		}
		return nil
	}
	if err := checkNoMsg(); err != nil {
		return err
	}

	testData := "test"
	checkMsg := func(m mqueue.Message) error {
		if !bytes.Equal(m.GetData(), []byte(testData)) {
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
	if err != nil {
		return err
	}
	if !ok || peekedMsg == nil {
		return errors.New("expected peek() to be ok after push()")
	}
	err = checkMsg(peekedMsg)
	if err != nil {
		return err
	}

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

	// Extra tests
	id := []byte("test-mqueue")
	mq, err = ktx.OpenMqueue(ctx, id)
	if err != nil {
		return err
	}
	srcData := func() []byte {
		return []byte("hello world")
	}
	msg, err := mq.Push(srcData())
	if err != nil {
		return err
	}
	if !bytes.Equal(msg.GetData(), srcData()) {
		return errors.Errorf("expected %v got %v", srcData(), msg.GetData())
	}
	m2, err := mq.Wait(ctx, false)
	if err != nil {
		return err
	}
	if !bytes.Equal(m2.GetData(), srcData()) {
		return errors.Errorf("expected %v got %v", srcData(), m2.GetData())
	}
	if m2.GetId() != msg.GetId() {
		return errors.Errorf("expected id %v got %v", m2.GetId(), msg.GetId())
	}
	m3, ok, err := mq.Peek()
	if !ok {
		return errors.New("expected peek to get msg, but !ok")
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(m3.GetData(), srcData()) {
		return errors.Errorf("expected %v got %v", srcData(), m3.GetData())
	}
	if m3.GetId() != msg.GetId() {
		return errors.Errorf("expected %v got %v", m3.GetId(), msg.GetId())
	}
	if err := mq.DeleteQueue(); err != nil {
		return err
	}
	_, ok, _ = mq.Peek()
	if ok {
		return errors.New("expected !ok after delete queue, got ok")
	}
	return nil
}
