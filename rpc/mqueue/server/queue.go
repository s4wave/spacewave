package rpc_mqueue_server

import (
	"context"

	rpc_mqueue "github.com/aperturerobotics/bldr/rpc/mqueue"
	"github.com/aperturerobotics/hydra/mqueue"
)

// Queue implements the message queue.
type Queue struct {
	// queue is the kvtx message queue
	queue mqueue.Queue
}

// NewQueue constructs a new KvtxQueue service.
func NewQueue(queue mqueue.Queue) *Queue {
	return &Queue{queue: queue}
}

// Peek peeks the next value in the queue without removing it.
func (q *Queue) Peek(ctx context.Context, req *rpc_mqueue.PeekRequest) (*rpc_mqueue.PeekResponse, error) {
	msg, found, err := q.queue.Peek()
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_mqueue.PeekResponse{
		Msg:   rpc_mqueue.ToMqueueMsg(msg),
		Found: found,
		Error: errStr,
	}, nil
}

// Ack acknowledges a message by ID if the ID is the current message at the front of the queue.
func (q *Queue) Ack(ctx context.Context, req *rpc_mqueue.AckRequest) (*rpc_mqueue.AckResponse, error) {
	err := q.queue.Ack(req.GetId())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_mqueue.AckResponse{Error: errStr}, nil
}

// Push pushes a message to the queue.
func (q *Queue) Push(ctx context.Context, req *rpc_mqueue.PushRequest) (*rpc_mqueue.PushResponse, error) {
	msg, err := q.queue.Push(req.GetData())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_mqueue.PushResponse{
		Error: errStr,
		Msg:   rpc_mqueue.ToMqueueMsg(msg),
	}, nil
}

// Wait waits for a message to arrive.
func (q *Queue) Wait(ctx context.Context, req *rpc_mqueue.WaitRequest) (*rpc_mqueue.WaitResponse, error) {
	ack := req.GetAck()
	msg, err := q.queue.Wait(ctx, ack)
	if err != nil {
		return nil, err
	}
	return &rpc_mqueue.WaitResponse{
		Msg: rpc_mqueue.ToMqueueMsg(msg),
	}, nil
}

// DeleteQueue deletes a queue and its contents.
func (q *Queue) DeleteQueue(ctx context.Context, req *rpc_mqueue.DeleteQueueRequest) (*rpc_mqueue.DeleteQueueResponse, error) {
	err := q.queue.DeleteQueue()
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &rpc_mqueue.DeleteQueueResponse{Error: errStr}, nil
}

// _ is a type assertion
var _ rpc_mqueue.SRPCQueueOpsServer = ((*Queue)(nil))
