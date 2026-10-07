package block_gc

// refBatchError reports a failed ownership transition and its uncommitted suffix.
// The slices retain the original add-before-remove order.
type refBatchError struct {
	// err is the underlying preparation or commit failure.
	err error
	// adds holds additions that have not committed.
	adds []RefEdge
	// removes holds removals that have not committed.
	removes []RefEdge
}

// Error returns the underlying failure's message.
func (e *refBatchError) Error() string {
	return e.err.Error()
}

// Unwrap exposes the underlying failure for error matching.
func (e *refBatchError) Unwrap() error {
	return e.err
}

// RefBatchRemainder returns the ownership changes to retain for another attempt.
func (e *refBatchError) RefBatchRemainder() ([]RefEdge, []RefEdge) {
	return e.adds, e.removes
}

// _ is a type assertion.
var _ RefBatchRemainderError = (*refBatchError)(nil)
