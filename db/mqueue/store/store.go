package mqueue_store

import (
	"context"

	"github.com/s4wave/spacewave/db/mqueue"
)

// Store implements the message queue store.
type Store interface {
	// ListMessageQueues lists message queues with a given ID prefix.
	//
	// Note: if !filled, implementation might not return queues that are empty.
	// If filled is set, implementation must only return filled queues.
	ListMessageQueues(ctx context.Context, prefix []byte, filled bool) ([][]byte, error)
	// OpenMqueue opens a message queue by ID.
	//
	// If the message queue does not exist, creates it.
	OpenMqueue(ctx context.Context, id []byte) (mqueue.Queue, error)
	// DelMqueue deletes a mqueue by ID.
	//
	// If not found, should not return an error.
	DelMqueue(ctx context.Context, id []byte) error
}
