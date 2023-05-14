package hydra_git

import (
	"context"

	"github.com/aperturerobotics/hydra/tx"
	"github.com/go-git/go-git/v5/storage"
)

// Storer is the interface for storing Git repository data.
type Storer interface {
	// Storer is the go-git storage interface.
	storage.Storer
	// GetReadOnly returns if the state is read-only.
	GetReadOnly() bool
}

// Tx implements a storer as a transaction.
type Tx interface {
	// Tx contains the commit and discard funcs.
	tx.Tx
	// Storer implements the Git storage.
	Storer
}

// Engine is the interface for a transactional Git engine.
type Engine interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	// Check GetReadOnly, might not return a write tx if write=true.
	// ctx is only used when constructing the transaction.
	NewTransaction(ctx context.Context, write bool) (Tx, error)
}
