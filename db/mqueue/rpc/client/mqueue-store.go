package mqueue_rpc_client

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/mqueue"
	mqueue_rpc "github.com/s4wave/spacewave/db/mqueue/rpc"
	mqueue_store "github.com/s4wave/spacewave/db/mqueue/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// MqueueStore implements a message queue store backed by a MqueueStore service.
type MqueueStore struct {
	// client is the client to use
	client mqueue_rpc.SRPCMqueueStoreClient
}

// NewMqueueStore constructs a new MqueueStore.
func NewMqueueStore(client mqueue_rpc.SRPCMqueueStoreClient) *MqueueStore {
	return &MqueueStore{client: client}
}

// OpenMqueue opens a message queue by ID.
// The context is used for the API calls.
func (s *MqueueStore) OpenMqueue(ctx context.Context, id []byte) (mqueue.Queue, error) {
	// note: we assume that message queue IDs can be cast to a string here.
	// this is not always the case, maybe rpcStream should accept []byte instead?
	idAsStr := string(id)
	openStream := rpcstream.NewRpcStreamOpenStream(s.client.MqueueRpc, idAsStr, false)
	rpcClient := srpc.NewClient(openStream)
	queueClient := mqueue_rpc.NewSRPCQueueOpsClient(rpcClient)
	mqueueClient := NewQueue(queueClient)
	return mqueueClient, nil
}

// ListMessageQueues lists message queues with a given ID prefix.
//
// Note: if !filled, implementation might not return queues that are empty.
// If filled is set, implementation must only return filled queues.
func (s *MqueueStore) ListMessageQueues(ctx context.Context, prefix []byte, filled bool) ([][]byte, error) {
	resp, err := s.client.ListMqueues(ctx, &mqueue_rpc.ListMqueuesRequest{
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
	resp, err := s.client.RmMqueue(ctx, &mqueue_rpc.RmMqueueRequest{
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
