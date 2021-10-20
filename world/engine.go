package world

// Engine implements a transactional world state container.
type Engine interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	// Check GetReadOnly, might not return a write tx if write=true.
	NewTransaction(write bool) (Tx, error)

	// WorldWait allows waiting for the world state to change.
	WorldWait
	// WorldStorage provides access to the world storage via bucket cursors.
	WorldStorage
}
