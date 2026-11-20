package store_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Store extends the kvtx store with an execute func.
type Store interface {
	kvtx.Store
	// Execute runs the store's background work until ctx is canceled.
	// Returning nil ends that work and leaves the store in use. Returning an
	// error stops the volume holding the store: the volume controller returns
	// the error from its own Execute.
	Execute(ctx context.Context) error
}
