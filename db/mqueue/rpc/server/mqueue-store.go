package mqueue_rpc_server

import (
	"context"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	mqueue_rpc "github.com/s4wave/spacewave/db/mqueue/rpc"
	mqueue_store "github.com/s4wave/spacewave/db/mqueue/store"
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
	strm mqueue_rpc.SRPCMqueueStore_MqueueRpcStream,
) error {
	return rpcstream.HandleRpcStream(strm, s.GetMqueueMux)
}

// GetMqueueMux returns the srpc.Mux for a message queue.
func (s *MqueueStore) GetMqueueMux(ctx context.Context, mqueueID string, _ func()) (srpc.Invoker, func(), error) {
	queue, err := s.store.OpenMqueue(ctx, []byte(mqueueID))
	if err != nil {
		return nil, nil, err
	}
	mux := srpc.NewMux()
	if err := mqueue_rpc.SRPCRegisterQueueOps(mux, NewQueue(queue)); err != nil {
		return nil, nil, err
	}
	return mux, nil, nil
}

// ListMqueues lists the message queues in the store.
func (s *MqueueStore) ListMqueues(ctx context.Context, req *mqueue_rpc.ListMqueuesRequest) (*mqueue_rpc.ListMqueuesResponse, error) {
	mqueueIDs, err := s.store.ListMessageQueues(ctx, req.GetPrefix(), req.GetFilled())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &mqueue_rpc.ListMqueuesResponse{Error: errStr, MqueueIds: mqueueIDs}, nil
}

// RmMqueue attempts to remove a message queue.
func (s *MqueueStore) RmMqueue(ctx context.Context, req *mqueue_rpc.RmMqueueRequest) (*mqueue_rpc.RmMqueueResponse, error) {
	err := s.store.DelMqueue(ctx, req.GetMqueueId())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &mqueue_rpc.RmMqueueResponse{Error: errStr}, nil
}

// _ is a type assertion
var _ mqueue_rpc.SRPCMqueueStoreServer = ((*MqueueStore)(nil))
