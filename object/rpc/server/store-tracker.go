package object_rpc_server

import (
	"context"

	"github.com/aperturerobotics/controllerbus/util/ccontainer"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	rpc_kvtx_server "github.com/aperturerobotics/hydra/kvtx/rpc/server"
)

// kvtxStoreTracker tracks a kvtx.Store
type kvtxStoreTracker struct {
	// s is the parent store
	s *ObjectStore
	// objectStoreID is the object store identifier.
	objectStoreID string
	// storeCtr contains the proxy object store
	// set when the store is ready to use
	storeCtr *ccontainer.CContainer[*rpc_kvtx_server.Store]
	// errCtr contains any error fetching the store.
	errCtr *ccontainer.CContainer[*error]
}

// newKvtxStoreTracker constructs a new tracker routine.
func (s *ObjectStore) newKvtxStoreTracker(key string) (keyed.Routine, *kvtxStoreTracker) {
	tr := &kvtxStoreTracker{
		s:             s,
		objectStoreID: key,
		storeCtr:      ccontainer.NewCContainer[*rpc_kvtx_server.Store](nil),
		errCtr:        ccontainer.NewCContainer[*error](nil),
	}
	return tr.execute, tr
}

// kvtxStoreTrackerExited handles execute() returning an error.
func (s *ObjectStore) kvtxStoreTrackerExited(key string, routine keyed.Routine, t *kvtxStoreTracker, err error) {
	if err != nil {
		t.errCtr.SetValue(&err)
	}
}

// execute executes the proxy volume tracker.
func (t *kvtxStoreTracker) execute(ctx context.Context) error {
	objectStoreID := t.objectStoreID
	objStore, err := t.s.store.OpenObjectStore(ctx, objectStoreID)
	if err != nil {
		return err
	}
	objStoreRpc := rpc_kvtx_server.NewStore(objStore)
	t.storeCtr.SetValue(objStoreRpc)
	return nil
}

// waitStore waits for the ObjectStore to be opened or an error to occur.
func (t *kvtxStoreTracker) waitStore(rctx context.Context) (*rpc_kvtx_server.Store, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	errCh := make(chan error, 1)
	go func() {
		rerr, err := t.errCtr.WaitValue(ctx, nil)
		if err == nil && rerr != nil {
			err = *rerr
		}
		if err != nil {
			errCh <- err
		}
	}()
	val, err := t.storeCtr.WaitValue(ctx, errCh)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, nil
	}
	return val, nil
}
