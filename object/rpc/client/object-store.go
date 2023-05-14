package object_rpc_client

import (
	"context"
	"errors"

	rpc_kvtx "github.com/aperturerobotics/hydra/kvtx/rpc"
	rpc_kvtx_client "github.com/aperturerobotics/hydra/kvtx/rpc/client"
	"github.com/aperturerobotics/hydra/object"
	object_rpc "github.com/aperturerobotics/hydra/object/rpc"
	object_store "github.com/aperturerobotics/hydra/object/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// ObjectStore implements a ObjectStore backed by a ObjectStore service.
type ObjectStore struct {
	// client is the client to use
	client object_rpc.SRPCObjectStoreClient
}

// NewObjectStore constructs a new ObjectStore.
func NewObjectStore(client object_rpc.SRPCObjectStoreClient) *ObjectStore {
	return &ObjectStore{client: client}
}

// OpenObjectStore opens a object store by ID.
// The context is used for the API calls.
func (s *ObjectStore) OpenObjectStore(ctx context.Context, id string) (object.ObjectStore, error) {
	openStream := rpcstream.NewRpcStreamOpenStream(s.client.ObjectStoreRpc, id, false)
	rpcClient := srpc.NewClient(openStream)
	storeClient := rpc_kvtx.NewSRPCKvtxClient(rpcClient)
	objStoreClient := rpc_kvtx_client.NewStore(ctx, storeClient)
	return objStoreClient, nil
}

// RmObjectStore deletes a object store and all contents by ID.
func (s *ObjectStore) RmObjectStore(ctx context.Context, id string) error {
	resp, err := s.client.RmObjectStore(ctx, &object_rpc.RmObjectStoreRequest{
		ObjectStoreId: id,
	})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// _ is a type assertion
var _ object_store.Store = ((*ObjectStore)(nil))
