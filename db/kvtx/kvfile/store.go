// Package kvtx_kvfile implements a key/value store backed by a file. The file is
// written with the values at the beginning and an index of the keys at the end.
// The design allows for fast on-demand lookups via an index binary search.
package kvtx_kvfile

import (
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/kvtx"
)

// KvfileStore implements a read-only Store backed by a kvfile.
//
// While write transactions can be created, any write operation will fail.
type KvfileStore struct {
	rdr *kvfile.Reader
}

// NewKvfileStore constructs a new KvfileStore.
func NewKvfileStore(rdr *kvfile.Reader) *KvfileStore {
	return &KvfileStore{rdr: rdr}
}

// GetKvfileReader returns the inner kvfile reader.
func (s *KvfileStore) GetKvfileReader() *kvfile.Reader {
	return s.rdr
}

// NewTransaction returns a new transaction against the store.
// The transaction will be read-only regardless of write.
func (s *KvfileStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return NewTransaction(s.rdr, write), nil
}

// _ is a type assertion
var _ kvtx.Store = ((*KvfileStore)(nil))
