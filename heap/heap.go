package heap

import "context"

// Heap implements a kvtx-backed heap.
type Heap interface {
	// IsEmpty checks if the heap is empty.
	IsEmpty(ctx context.Context) (bool, error)
	// Size returns the number of elements in the heap.
	Size(ctx context.Context) (uint64, error)
	// Min returns the minimum element and priority in the heap.
	Min(ctx context.Context) ([]byte, float64, error)
	// DequeueMin removes and returns the lowest element.
	DequeueMin(ctx context.Context) (rmin []byte, pmin float64, rerr error)
	// DecreaseKey decreases the priority of the given element.
	// Note: returns error if not found or if priority is higher than given.
	DecreaseKey(ctx context.Context, key []byte, newPriority float64) (rerr error)
	// Enqueue enqueues the key with the given priority.
	//
	// If exists, updates the priority to the new value.
	Enqueue(ctx context.Context, key []byte, priority float64) error
	// Lookup checks priority of the given key.
	// Returns 0, false, nil if not found.
	Lookup(ctx context.Context, key []byte) (float64, bool, error)
	// Flush deletes all elements in the heap.
	Flush(ctx context.Context) error
	// Delete deletes an element from the heap.
	// No error is returned if not found.
	Delete(ctx context.Context, key []byte) (rerr error)
}
