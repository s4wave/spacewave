package store_test

import (
	"context"

	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/store"
	"github.com/sirupsen/logrus"
)

// TestAll runs all tests.
func TestAll(ctx context.Context, ktx store.Store) error {
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
