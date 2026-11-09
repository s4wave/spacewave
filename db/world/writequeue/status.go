package writequeue

// Status describes work waiting to fork or waiting for admission/durability.
type Status struct {
	// Pending counts writes waiting for a batch, with those parked to fetch
	// blocks from peers.
	Pending int
	// Committing counts batches whose durable receipts are outstanding.
	Committing int
}

// Status returns bounded queue diagnostics without scanning its writes.
func (q *Queue) Status() Status {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	return Status{Pending: len(q.pending) + len(q.parked), Committing: len(q.committing)}
}
