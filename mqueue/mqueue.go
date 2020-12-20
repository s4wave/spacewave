package mqueue

import (
	"context"
	"time"
)

// Queue is a store-backed, FIFO, at-least-once delivery, concurrency safe
// message queue. It can be implemented with various algorithms given the
// underlying store implementation.
type Queue interface {
	// Peek returns the next message, if any.
	Peek() (Message, bool, error)
	// Ack acknowledges the head message by ID, if the head message matches the
	// given match ID.
	Ack(id uint64) error
	// Push pushes a message to the queue.
	// Note: The data buffer may be reused for GetData() in the message.
	Push(data []byte) (Message, error)
	// Wait() waits for the next message, or context cancellation.
	//
	// Returns the message. Equiv to Peek if a message is available.
	// Acks the message immediately if ack is true.
	Wait(ctx context.Context, ack bool) (Message, error)
	// DeleteQueue deletes all messages and metadata from the queue.
	DeleteQueue() error
}

// Message is a message in the queue.
type Message interface {
	// GetId returns the numeric message identifier.
	GetId() uint64
	// GetTimestamp returns the message timestamp.
	GetTimestamp() time.Time
	// GetData returns the inner message data.
	GetData() []byte
}
