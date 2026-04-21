package store_test

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/mqueue"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/store"
	"github.com/sirupsen/logrus"
)

// TestAll runs all tests.
func TestAll(ctx context.Context, ktx store.Store) error {
	if err := TestMqueueAPI(ctx, ktx); err != nil {
		return err
	}
	if err := TestObjectStore(ctx, ktx); err != nil {
		return err
	}
	return nil
}

// WithVLogger attaches a vlogger to the object store.
func WithVLogger(le *logrus.Entry) func(objStore object.ObjectStore) (object.ObjectStore, error) {
	return func(objStore object.ObjectStore) (object.ObjectStore, error) {
		return kvtx_vlogger.NewVLogger(le, objStore), nil
	}
}

// TestObjectStoreFn is a test function for TestObjectStore.
type TestObjectStoreFn func(objStore object.ObjectStore) (object.ObjectStore, error)

// TestObjectStore tests the object store.
func TestObjectStore(
	rctx context.Context,
	ktx store.Store,
	cbs ...TestObjectStoreFn,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	obj, relObj, err := ktx.AccessObjectStore(ctx, "test-store-2", ctxCancel)
	if err != nil {
		return err
	}
	defer relObj()

	for _, cb := range cbs {
		nextStore, err := cb(obj)
		if err != nil {
			return err
		}
		if nextStore != nil {
			obj = nextStore
		}
	}

	if err := kvtx_kvtest.TestAll(ctx, obj); err != nil {
		return err
	}

	if err := ktx.DeleteObjectStore(ctx, "test-store-2"); err != nil {
		return err
	}

	return nil
}

// TestReconcilerMqueueE2E tests the reconciler event queue end to end.
func TestReconcilerMqueue(ctx context.Context, ktx store.Store) error {
	pair := bucket_store.BucketReconcilerPair{
		BucketID:     "test-bucket-reconciler",
		ReconcilerID: "test-reconciler",
	}
	mq, err := ktx.GetReconcilerEventQueue(ctx, pair)
	if err != nil {
		return err
	}

	checkNoMsg := func() error {
		msg, ok, err := mq.Peek(ctx)
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
	pushedMsg, err := mq.Push(ctx, []byte(testData))
	if err != nil {
		return err
	}
	if err := checkMsg(pushedMsg); err != nil {
		return err
	}

	pairs, err := ktx.ListFilledReconcilerEventQueues(ctx)
	if err != nil {
		return err
	}
	if len(pairs) != 1 {
		return errors.New("expected 1 pair")
	}

	peekedMsg, ok, err := mq.Peek(ctx)
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

	if err := mq.Ack(ctx, peekedMsg.GetId()); err != nil {
		return err
	}
	if err := checkNoMsg(); err != nil {
		return err
	}

	pushedMsg, err = mq.Push(ctx, []byte(testData))
	if err != nil {
		return err
	}
	if err := checkMsg(pushedMsg); err != nil {
		return err
	}

	if err := ktx.DeleteReconcilerEventQueue(ctx, pair); err != nil {
		return err
	}
	if err := checkNoMsg(); err != nil {
		return err
	}

	return nil
}

// TestMqueueAPI tests the message queue api.
func TestMqueueAPI(rctx context.Context, ktx store.Store) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Extra tests
	id := []byte("test-mqueue")
	mq, err := ktx.OpenMqueue(ctx, id)
	if err != nil {
		return err
	}
	srcData := func() []byte {
		return []byte("hello world")
	}
	msg, err := mq.Push(ctx, srcData())
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
	m3, ok, err := mq.Peek(ctx)
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
	if err := mq.DeleteQueue(ctx); err != nil {
		return err
	}
	_, ok, _ = mq.Peek(ctx)
	if ok {
		return errors.New("expected !ok after delete queue, got ok")
	}
	return nil
}
