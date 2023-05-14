package tx

import "context"

// Tx is a generic transaction interface.
type Tx interface {
	// Commit commits the transaction to storage.
	// Can return an error to indicate tx failure.
	Commit(ctx context.Context) error
	// Discard cancels the transaction.
	// If called after Commit, does nothing.
	// Cannot return an error.
	// Can be called unlimited times.
	// Always call Discard or Commit when done with a tx.
	Discard()
}
