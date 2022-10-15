package rpc_object_client

import (
	"context"
	"errors"

	rpc_kvtx "github.com/aperturerobotics/bldr/rpc/kvtx"
	rpc_kvtx_client "github.com/aperturerobotics/bldr/rpc/kvtx/client"
	rpc_object "github.com/aperturerobotics/bldr/rpc/object"
	"github.com/aperturerobotics/hydra/object"
	object_store "github.com/aperturerobotics/hydra/object/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// ObjectStore implements a ObjectStore backed by a ObjectStore service.
type ObjectStore struct {
	// ctx is used for volume lookups
	ctx context.Context
	// client is the client to use
	client rpc_object.SRPCObjectStoreClient
}

// NewObjectStore constructs a new ObjectStore.
func NewObjectStore(
	ctx context.Context,
	client rpc_object.SRPCObjectStoreClient,
) *ObjectStore {
	return &ObjectStore{
		ctx:    ctx,
		client: client,
	}
}

// OpenObjectStore opens a object store by ID.
// The context is used for the API calls.
func (s *ObjectStore) OpenObjectStore(ctx context.Context, id string) (object.ObjectStore, error) {
	openStream := rpcstream.NewRpcStreamOpenStream(s.client.ObjectStoreRpc, id)
	rpcClient := srpc.NewClient(openStream)
	storeClient := rpc_kvtx.NewSRPCKvtxClient(rpcClient)
	objStoreClient := rpc_kvtx_client.NewStore(ctx, storeClient)
	return objStoreClient, nil
}

// RmObjectStore deletes a object store and all contents by ID.
func (s *ObjectStore) RmObjectStore(ctx context.Context, id string) error {
	resp, err := s.client.RmObjectStore(ctx, &rpc_object.RmObjectStoreRequest{
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
