package object

import (
	"errors"

	"github.com/aperturerobotics/hydra/kvtx"
)

// ErrObjectStoreClosed is returned if the store is closed.
var ErrObjectStoreClosed = errors.New("object store is closed")

// ObjectStore implements a key/value object store.
// Calls may return ErrObjectStoreClosed or context.Canceled if the store is closed.
type ObjectStore interface {
	// Store indicates ObjectStore provides a transactional key/value store.
	kvtx.Store
}
