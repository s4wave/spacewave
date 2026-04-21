package object_rpc_client

import (
	"context"
	"errors"

	rpc_kvtx "github.com/s4wave/spacewave/db/kvtx/rpc"
	rpc_kvtx_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	"github.com/s4wave/spacewave/db/object"
	object_rpc "github.com/s4wave/spacewave/db/object/rpc"
	object_store "github.com/s4wave/spacewave/db/object/store"
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

// AccessObjectStore opens a object store by ID.
// The context is used for the API calls.
func (s *ObjectStore) AccessObjectStore(ctx context.Context, id string, released func()) (object.ObjectStore, func(), error) {
	openStream := rpcstream.NewRpcStreamOpenStream(s.client.ObjectStoreRpc, id, false)
	rpcClient := srpc.NewClient(openStream)
	storeClient := rpc_kvtx.NewSRPCKvtxClient(rpcClient)
	objStoreClient := rpc_kvtx_client.NewStore(storeClient)
	return objStoreClient, func() {}, nil
}

// DeleteObjectStore deletes a object store and all contents by ID.
func (s *ObjectStore) DeleteObjectStore(ctx context.Context, id string) error {
	resp, err := s.client.DeleteObjectStore(ctx, &object_rpc.DeleteObjectStoreRequest{
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
