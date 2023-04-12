package store_kvtx_kvfile

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_kvfile "github.com/aperturerobotics/hydra/kvtx/kvfile"
	"github.com/aperturerobotics/go-kvfile"
)

// Store is a read-only kvfile store.
//
// Uses the kvfile file format.
// If write=true, uses an in-memory key/value store as an overlay.
type Store struct {
	*kvtx_kvfile.KvfileStore
}

// NewStore constructs a new key-value store.
func NewStore(rdr *kvfile.Reader) *Store {
	return &Store{KvfileStore: kvtx_kvfile.NewKvfileStore(rdr)}
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
