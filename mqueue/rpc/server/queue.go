package mqueue_rpc_server

import (
	"context"

	"github.com/aperturerobotics/hydra/mqueue"
	mqueue_rpc "github.com/aperturerobotics/hydra/mqueue/rpc"
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
func (q *Queue) Peek(ctx context.Context, req *mqueue_rpc.PeekRequest) (*mqueue_rpc.PeekResponse, error) {
	msg, found, err := q.queue.Peek(ctx)
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &mqueue_rpc.PeekResponse{
		Msg:   mqueue_rpc.ToMqueueMsg(msg),
		Found: found,
		Error: errStr,
	}, nil
}

// Ack acknowledges a message by ID if the ID is the current message at the front of the queue.
func (q *Queue) Ack(ctx context.Context, req *mqueue_rpc.AckRequest) (*mqueue_rpc.AckResponse, error) {
	err := q.queue.Ack(ctx, req.GetId())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &mqueue_rpc.AckResponse{Error: errStr}, nil
}

// Push pushes a message to the queue.
func (q *Queue) Push(ctx context.Context, req *mqueue_rpc.PushRequest) (*mqueue_rpc.PushResponse, error) {
	msg, err := q.queue.Push(ctx, req.GetData())
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &mqueue_rpc.PushResponse{
		Error: errStr,
		Msg:   mqueue_rpc.ToMqueueMsg(msg),
	}, nil
}

// Wait waits for a message to arrive.
func (q *Queue) Wait(ctx context.Context, req *mqueue_rpc.WaitRequest) (*mqueue_rpc.WaitResponse, error) {
	ack := req.GetAck()
	msg, err := q.queue.Wait(ctx, ack)
	if err != nil {
		return nil, err
	}
	return &mqueue_rpc.WaitResponse{
		Msg: mqueue_rpc.ToMqueueMsg(msg),
	}, nil
}

// DeleteQueue deletes a queue and its contents.
func (q *Queue) DeleteQueue(ctx context.Context, req *mqueue_rpc.DeleteQueueRequest) (*mqueue_rpc.DeleteQueueResponse, error) {
	err := q.queue.DeleteQueue(ctx)
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	return &mqueue_rpc.DeleteQueueResponse{Error: errStr}, nil
}

// _ is a type assertion
var _ mqueue_rpc.SRPCQueueOpsServer = ((*Queue)(nil))
