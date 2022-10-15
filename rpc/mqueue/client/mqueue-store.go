package rpc_mqueue_client

import (
	"context"
	"errors"

	rpc_mqueue "github.com/aperturerobotics/bldr/rpc/mqueue"
	"github.com/aperturerobotics/hydra/mqueue"
	mqueue_store "github.com/aperturerobotics/hydra/mqueue/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// MqueueStore implements a message queue store backed by a MqueueStore service.
type MqueueStore struct {
	// ctx is used for volume lookups
	ctx context.Context
	// client is the client to use
	client rpc_mqueue.SRPCMqueueStoreClient
}

// NewMqueueStore constructs a new MqueueStore.
func NewMqueueStore(
	ctx context.Context,
	client rpc_mqueue.SRPCMqueueStoreClient,
) *MqueueStore {
	return &MqueueStore{
		ctx:    ctx,
		client: client,
	}
}

// OpenMqueue opens a message queue by ID.
// The context is used for the API calls.
func (s *MqueueStore) OpenMqueue(ctx context.Context, id []byte) (mqueue.Queue, error) {
	// note: we assume that message queue IDs can be cast to a string here.
	// this is not always the case, maybe rpcStream should accept []byte instead?
	idAsStr := string(id)
	openStream := rpcstream.NewRpcStreamOpenStream(s.client.MqueueRpc, idAsStr)
	rpcClient := srpc.NewClient(openStream)
	queueClient := rpc_mqueue.NewSRPCQueueOpsClient(rpcClient)
	mqueueClient := NewQueue(ctx, queueClient)
	return mqueueClient, nil
}

// ListMessageQueues lists message queues with a given ID prefix.
//
// Note: if !filled, implementation might not return queues that are empty.
// If filled is set, implementation must only return filled queues.
func (s *MqueueStore) ListMessageQueues(prefix []byte, filled bool) ([][]byte, error) {
	resp, err := s.client.ListMqueues(s.ctx, &rpc_mqueue.ListMqueuesRequest{
		Prefix: prefix,
		Filled: filled,
	})
	if err := s.err(err, resp.GetError()); err != nil {
		return nil, err
	}
	return resp.GetMqueueIds(), nil
}

// DelMqueue deletes a message queue and all contents by ID.
//
// If not found, should not return an error.
func (s *MqueueStore) DelMqueue(ctx context.Context, id []byte) error {
	resp, err := s.client.RmMqueue(ctx, &rpc_mqueue.RmMqueueRequest{
		MqueueId: id,
	})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// err converts an error into the appropriate error.
func (s *MqueueStore) err(err error, errStr string) error {
	if err == nil {
		if errStr != "" {
			err = errors.New(errStr)
		} else {
			return nil
		}
	}
	return err
}

// _ is a type assertion
var _ mqueue_store.Store = ((*MqueueStore)(nil))
