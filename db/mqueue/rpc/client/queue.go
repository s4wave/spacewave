package mqueue_rpc_client

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/mqueue"
	mqueue_rpc "github.com/s4wave/spacewave/db/mqueue/rpc"
)

// Queue implements Queue with a QueueOps service.
type Queue struct {
	// client is the service client
	client mqueue_rpc.SRPCQueueOpsClient
}

// NewQueue constructs a new TxQueue.
func NewQueue(client mqueue_rpc.SRPCQueueOpsClient) *Queue {
	return &Queue{
		client: client,
	}
}

// Peek returns the next message, if any.
func (q *Queue) Peek(ctx context.Context) (mqueue.Message, bool, error) {
	resp, err := q.client.Peek(ctx, &mqueue_rpc.PeekRequest{})
	if err != nil {
		return nil, false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, false, errors.New(errStr)
	}
	if !resp.GetFound() {
		return nil, false, nil
	}
	return mqueue_rpc.NewMsg(resp.GetMsg()), true, nil
}

// Ack acknowledges the head message by ID, if the head message matches the
// given match ID.
func (q *Queue) Ack(ctx context.Context, id uint64) error {
	resp, err := q.client.Ack(ctx, &mqueue_rpc.AckRequest{
		Id: id,
	})
	if err := q.err(err, resp.GetError()); err != nil {
		return err
	}
	return nil
}

// Push pushes a message to the queue.
// Note: The data buffer may be reused for GetData() in the message.
func (q *Queue) Push(ctx context.Context, data []byte) (mqueue.Message, error) {
	resp, err := q.client.Push(ctx, &mqueue_rpc.PushRequest{
		Data: data,
	})
	if err := q.err(err, resp.GetError()); err != nil {
		return nil, err
	}
	respMsg := resp.GetMsg()
	if respMsg != nil && len(respMsg.Data) == 0 {
		respMsg.Data = data
	}
	return mqueue_rpc.NewMsg(respMsg), nil
}

// Wait() waits for the next message, or context cancellation.
//
// Returns the message. Equiv to Peek if a message is available.
// Acks the message immediately if ack is true.
func (q *Queue) Wait(ctx context.Context, ack bool) (mqueue.Message, error) {
	resp, err := q.client.Wait(ctx, &mqueue_rpc.WaitRequest{Ack: ack})
	if err != nil {
		return nil, err
	}
	respMsg := resp.GetMsg()
	return mqueue_rpc.NewMsg(respMsg), nil
}

// DeleteQueue deletes all messages and metadata from the queue.
func (q *Queue) DeleteQueue(ctx context.Context) error {
	resp, err := q.client.DeleteQueue(ctx, &mqueue_rpc.DeleteQueueRequest{})
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
