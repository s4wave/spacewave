package object_rpc_server

import (
	"context"

	rpc_kvtx_server "github.com/s4wave/spacewave/db/kvtx/rpc/server"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// kvtxStoreTracker tracks a kvtx.Store
type kvtxStoreTracker struct {
	// s is the parent store
	s *ObjectStore
	// objectStoreID is the object store identifier.
	objectStoreID string
	// storeCtr contains the proxy object store
	// set when the store is ready to use
	storeCtr *promise.PromiseContainer[*rpc_kvtx_server.Store]
}

// newKvtxStoreTracker constructs a new tracker routine.
func (s *ObjectStore) newKvtxStoreTracker(key string) (keyed.Routine, *kvtxStoreTracker) {
	tr := &kvtxStoreTracker{
		s:             s,
		objectStoreID: key,
		storeCtr:      promise.NewPromiseContainer[*rpc_kvtx_server.Store](),
	}
	return tr.execute, tr
}

// kvtxStoreTrackerExited handles kvStoreTracker returning an unexpected error.
func (s *ObjectStore) kvtxStoreTrackerExited(key string, routine keyed.Routine, t *kvtxStoreTracker, err error) {
	if err != nil {
		t.storeCtr.SetResult(nil, err)
	}
}

// execute executes the proxy volume tracker.
func (t *kvtxStoreTracker) execute(ctx context.Context) error {
	objectStoreID := t.objectStoreID
	sctx, sctxCancel := context.WithCancel(ctx)
	objStore, relObjStore, err := t.s.store.AccessObjectStore(ctx, objectStoreID, sctxCancel)
	if err != nil {
		return err
	}
	if relObjStore != nil {
		defer relObjStore()
	}

	objStoreRpc := rpc_kvtx_server.NewStore(objStore)
	t.storeCtr.SetResult(objStoreRpc, nil)
	defer t.storeCtr.SetPromise(nil)

	<-sctx.Done()
	return context.Canceled
}

// waitStore waits for the ObjectStore to be opened or an error to occur.
func (t *kvtxStoreTracker) waitStore(ctx context.Context) (*rpc_kvtx_server.Store, error) {
	return t.storeCtr.Await(ctx)
}
