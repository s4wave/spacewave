package object

import (
	"github.com/s4wave/spacewave/db/kvtx"
)

// ObjectStore implements a key/value object store.
// Calls may return ErrObjectStoreClosed or context.Canceled if the store is closed.
type ObjectStore interface {
	// Store indicates ObjectStore provides a transactional key/value store.
	kvtx.Store
}
