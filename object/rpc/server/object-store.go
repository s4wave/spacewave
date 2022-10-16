package object_rpc_server

import (
	"context"

	"github.com/aperturerobotics/controllerbus/util/keyed"
	rpc_kvtx "github.com/aperturerobotics/hydra/kvtx/rpc"
	object_rpc "github.com/aperturerobotics/hydra/object/rpc"
	object_store "github.com/aperturerobotics/hydra/object/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// ObjectStore implements the server with a ObjectStore.
type ObjectStore struct {
	// store is the underlying ObjectStore
	store object_store.Store
	// kvtxStores is the set of open object stores.
	kvtxStores *keyed.KeyedRefCount[*kvtxStoreTracker]
}

// NewObjectStore constructs a new ObjectStore.
func NewObjectStore(ctx context.Context, store object_store.Store) *ObjectStore {
	st := &ObjectStore{
		store: store,
	}
	st.kvtxStores = keyed.NewKeyedRefCount(
		st.newKvtxStoreTracker,
		st.kvtxStoreTrackerExited,
	)
	st.kvtxStores.SetContext(ctx, true)
	return st
}

// ObjectStoreRpc opens a RpcStream for a ObjectStore.
func (s *ObjectStore) ObjectStoreRpc(
	strm object_rpc.SRPCObjectStore_ObjectStoreRpcStream,
) error {
	return rpcstream.HandleRpcStream(strm, s.GetObjectStoreMux)
}

// RmObjectStore removes an object store by id.
func (s *ObjectStore) RmObjectStore(
	ctx context.Context,
	req *object_rpc.RmObjectStoreRequest,
) (*object_rpc.RmObjectStoreResponse, error) {
	err := s.store.RmObjectStore(ctx, req.GetObjectStoreId())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &object_rpc.RmObjectStoreResponse{Error: errStr}, nil
}

// GetObjectStoreMux returns the srpc.Mux for an object store.
func (s *ObjectStore) GetObjectStoreMux(ctx context.Context, objStoreID string) (srpc.Mux, func(), error) {
	ref, _ := s.kvtxStores.AddKeyRef(objStoreID)
	_, tracker := s.kvtxStores.GetKey(objStoreID)

	st, err := tracker.waitStore(ctx)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}

	mux := srpc.NewMux()
	if err := rpc_kvtx.SRPCRegisterKvtx(mux, st); err != nil {
		ref.Release()
		return nil, nil, err
	}
	return mux, ref.Release, nil
}

// _ is a type assertion
var _ object_rpc.SRPCObjectStoreServer = ((*ObjectStore)(nil))
