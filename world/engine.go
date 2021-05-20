package world

// Engine implements a transactional world state container.
type Engine interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	// If the store is not the latest HEAD block, it will be read-only.
	// Check GetReadOnly, might not return a write tx if write=true.
	NewTransaction(write bool) (Tx, error)
}
