package store_kvtx

import (
	"context"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Store extends the kvtx store with an execute func.
type Store interface {
	kvtx.Store
	// Execute executes the given store.
	// Returning nil ends execution.
	// Returning an error triggers a retry with backoff.
	Execute(ctx context.Context) error
}
