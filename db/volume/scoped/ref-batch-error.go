package volume_scoped

import block_gc "github.com/s4wave/spacewave/db/block/gc"

// batchError reports a failed ref batch with its uncommitted edges named in the
// view's own nodes, so the caller can retry them through the view.
type batchError struct {
	// err is the failure of the underlying graph.
	err error
	// adds holds the uncommitted additions.
	adds []block_gc.RefEdge
	// removes holds the uncommitted removals.
	removes []block_gc.RefEdge
}

// Error returns the message of the underlying failure.
func (e *batchError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying failure.
func (e *batchError) Unwrap() error {
	return e.err
}

// RefBatchRemainder returns the uncommitted additions and removals.
func (e *batchError) RefBatchRemainder() ([]block_gc.RefEdge, []block_gc.RefEdge) {
	return e.adds, e.removes
}

// _ is a type assertion
var _ block_gc.RefBatchRemainderError = (*batchError)(nil)
