package rpc_mqueue_client

import (
	"context"
	"errors"

	rpc_mqueue "github.com/aperturerobotics/bldr/rpc/mqueue"
	"github.com/aperturerobotics/hydra/mqueue"
)

// Queue implements Queue with a QueueOps service.
type Queue struct {
	// ctx is used for calls
	ctx context.Context
	// client is the service client
	client rpc_mqueue.SRPCQueueOpsClient
}

// NewQueue constructs a new TxQueue.
func NewQueue(ctx context.Context, client rpc_mqueue.SRPCQueueOpsClient) *Queue {
	return &Queue{
		ctx:    ctx,
		client: client,
	}
}

// Peek returns the next message, if any.
func (q *Queue) Peek() (mqueue.Message, bool, error) {
	resp, err := q.client.Peek(q.ctx, &rpc_mqueue.PeekRequest{})
	if err != nil {
		return nil, false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, false, errors.New(errStr)
	}
	if !resp.GetFound() {
		return nil, false, nil
	}
	return rpc_mqueue.NewMsg(resp.GetMsg()), true, nil
}

// Ack acknowledges the head message by ID, if the head message matches the
// given match ID.
func (q *Queue) Ack(id uint64) error {
	resp, err := q.client.Ack(q.ctx, &rpc_mqueue.AckRequest{
		Id: id,
	})
	if err := q.err(err, resp.GetError()); err != nil {
		return err
	}
	return nil
}

// Push pushes a message to the queue.
// Note: The data buffer may be reused for GetData() in the message.
func (q *Queue) Push(data []byte) (mqueue.Message, error) {
	resp, err := q.client.Push(q.ctx, &rpc_mqueue.PushRequest{
		Data: data,
	})
	if err := q.err(err, resp.GetError()); err != nil {
		return nil, err
	}
	respMsg := resp.GetMsg()
	if respMsg != nil && len(respMsg.Data) == 0 {
		respMsg.Data = data
	}
	return rpc_mqueue.NewMsg(respMsg), nil
}

// Wait() waits for the next message, or context cancellation.
//
// Returns the message. Equiv to Peek if a message is available.
// Acks the message immediately if ack is true.
func (q *Queue) Wait(ctx context.Context, ack bool) (mqueue.Message, error) {
	resp, err := q.client.Wait(ctx, &rpc_mqueue.WaitRequest{Ack: ack})
	if err != nil {
		return nil, err
	}
	respMsg := resp.GetMsg()
	return rpc_mqueue.NewMsg(respMsg), nil
}

// DeleteQueue deletes all messages and metadata from the queue.
func (q *Queue) DeleteQueue() error {
	resp, err := q.client.DeleteQueue(q.ctx, &rpc_mqueue.DeleteQueueRequest{})
	return q.err(err, resp.GetError())
}

// err converts an error into the appropriate error.
func (q *Queue) err(err error, errStr string) error {
	if err == nil && errStr != "" {
		err = errors.New(errStr)
	}
	return err
}

// _ is a type assertion
var _ mqueue.Queue = ((*Queue)(nil))
