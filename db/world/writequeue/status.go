package writequeue

// Status describes work waiting to fork or waiting for admission/durability.
type Status struct {
	// Pending counts writes waiting for a batch.
	Pending int
	// Committing counts batches whose durable receipts are outstanding.
	Committing int
}

// Status returns bounded queue diagnostics without scanning its writes.
func (q *Queue) Status() Status {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	return Status{Pending: len(q.pending), Committing: len(q.committing)}
}
