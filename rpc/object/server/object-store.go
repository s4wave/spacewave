package rpc_object_server

import (
	"context"

	rpc_kvtx "github.com/aperturerobotics/bldr/rpc/kvtx"
	rpc_kvtx_server "github.com/aperturerobotics/bldr/rpc/kvtx/server"
	rpc_object "github.com/aperturerobotics/bldr/rpc/object"
	object_store "github.com/aperturerobotics/hydra/object/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// ObjectStore implements the server with a ObjectStore.
type ObjectStore struct {
	// store is the underlying ObjectStore
	store object_store.Store
}

// NewObjectStore constructs a new ObjectStore.
func NewObjectStore(store object_store.Store) *ObjectStore {
	return &ObjectStore{store: store}
}

// ObjectStoreRpc opens a RpcStream for a ObjectStore.
func (s *ObjectStore) ObjectStoreRpc(
	strm rpc_object.SRPCObjectStore_ObjectStoreRpcStream,
) error {
	return rpcstream.HandleRpcStream(strm, s.GetObjectStoreMux)
}

// RmObjectStore removes an object store by id.
func (s *ObjectStore) RmObjectStore(
	ctx context.Context,
	req *rpc_object.RmObjectStoreRequest,
) (*rpc_object.RmObjectStoreResponse, error) {
	err := s.store.RmObjectStore(ctx, req.GetObjectStoreId())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_object.RmObjectStoreResponse{Error: errStr}, nil
}

// GetObjectStoreMux returns the srpc.Mux for an object store.
func (s *ObjectStore) GetObjectStoreMux(ctx context.Context, objStoreID string) (srpc.Mux, func(), error) {
	store, err := s.store.OpenObjectStore(ctx, objStoreID)
	if err != nil {
		return nil, nil, err
	}
	mux := srpc.NewMux()
	if err := rpc_kvtx.SRPCRegisterKvtx(mux, rpc_kvtx_server.NewStore(store)); err != nil {
		return nil, nil, err
	}
	return mux, nil, nil
}

// _ is a type assertion
var _ rpc_object.SRPCObjectStoreServer = ((*ObjectStore)(nil))
