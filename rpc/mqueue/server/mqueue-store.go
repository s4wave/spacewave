package rpc_mqueue_server

import (
	"context"

	rpc_mqueue "github.com/aperturerobotics/bldr/rpc/mqueue"
	mqueue_store "github.com/aperturerobotics/hydra/mqueue/store"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// MqueueStore implements the server with a MqueueStore.
type MqueueStore struct {
	// store is the underlying MqueueStore
	store mqueue_store.Store
}

// NewMqueueStore constructs a new MqueueStore.
func NewMqueueStore(store mqueue_store.Store) *MqueueStore {
	return &MqueueStore{store: store}
}

// MqueueRpc opens a RpcStream for a Queue.
func (s *MqueueStore) MqueueRpc(
	strm rpc_mqueue.SRPCMqueueStore_MqueueRpcStream,
) error {
	return rpcstream.HandleRpcStream(strm, s.GetMqueueMux)
}

// GetMqueueMux returns the srpc.Mux for a message queue.
func (s *MqueueStore) GetMqueueMux(ctx context.Context, mqueueID string) (srpc.Mux, func(), error) {
	queue, err := s.store.OpenMqueue(ctx, []byte(mqueueID))
	if err != nil {
		return nil, nil, err
	}
	mux := srpc.NewMux()
	if err := rpc_mqueue.SRPCRegisterQueueOps(mux, NewQueue(queue)); err != nil {
		return nil, nil, err
	}
	return mux, nil, nil
}

// ListMqueues lists the message queues in the store.
func (s *MqueueStore) ListMqueues(ctx context.Context, req *rpc_mqueue.ListMqueuesRequest) (*rpc_mqueue.ListMqueuesResponse, error) {
	mqueueIDs, err := s.store.ListMessageQueues(req.GetPrefix(), req.GetFilled())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_mqueue.ListMqueuesResponse{Error: errStr, MqueueIds: mqueueIDs}, nil
}

// RmMqueue attempts to remove a message queue.
func (s *MqueueStore) RmMqueue(ctx context.Context, req *rpc_mqueue.RmMqueueRequest) (*rpc_mqueue.RmMqueueResponse, error) {
	err := s.store.DelMqueue(ctx, req.GetMqueueId())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_mqueue.RmMqueueResponse{Error: errStr}, nil
}

// _ is a type assertion
var _ rpc_mqueue.SRPCMqueueStoreServer = ((*MqueueStore)(nil))
