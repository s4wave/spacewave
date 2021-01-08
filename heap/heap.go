package heap

// Heap implements a kvtx-backed heap.
type Heap interface {
	// IsEmpty checks if the heap is empty.
	IsEmpty() (bool, error)
	// Size returns the number of elements in the heap.
	Size() (int, error)
	// Min returns the minimum element and priority in the heap.
	Min() ([]byte, float64, error)
	// DequeueMin removes and returns the lowest element.
	DequeueMin() (rmin []byte, pmin float64, rerr error)
	// DecreaseKey decreases the priority of the given element.
	// Note: returns error if not found or if priority is higher than given.
	DecreaseKey(key []byte, newPriority float64) (rerr error)
	// Enqueue enqueues the key with the given priority.
	//
	// If exists, updates the priority to the new value.
	Enqueue(key []byte, priority float64) error
	// Flush deletes all elements in the heap.
	Flush() error
	// Delete deletes an element from the heap.
	// No error is returned if not found.
	Delete(key []byte) (rerr error)
}
