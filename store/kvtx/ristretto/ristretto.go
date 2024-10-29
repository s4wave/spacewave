package store_kvtx_ristretto

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/dgraph-io/ristretto/v2"
)

// Store is a ristretto cache backed kvtx store.
type Store struct {
	db  *ristretto.Cache[[]byte, []byte]
	ttl time.Duration
}

// NewStoreWithCache constructs a new key-value store from a cache.
func NewStoreWithCache(db *ristretto.Cache[[]byte, []byte], ttl time.Duration) *Store {
	return &Store{db: db}
}

// NewEmptyStore constructs a new empty ristretto cache store.
//
// conf can be nil
func NewStore(conf *Config) (*Store, error) {
	numCounters := int64(1e5)
	if cn := conf.GetNumCounters(); cn != 0 {
		numCounters = int64(cn)
	}

	maxCost := int64(1e9)
	if cn := conf.GetMaxCost(); cn != 0 {
		maxCost = int64(cn)
	}

	bufferItems := int64(64)
	if cn := conf.GetBufferItems(); cn != 0 {
		bufferItems = int64(cn)
	}

	ttlDur, err := conf.ParseTtlDur()
	if err != nil {
		return nil, err
	}

	db, err := ristretto.NewCache(&ristretto.Config[[]byte, []byte]{
		NumCounters: numCounters,
		MaxCost:     maxCost,
		BufferItems: bufferItems,
		Cost: func(value []byte) int64 {
			return int64(len(value))
		},
	})
	if err != nil {
		return nil, err
	}

	return NewStoreWithCache(db, ttlDur), nil
}

// GetCache returns the ristretto cache.
func (s *Store) GetCache() *ristretto.Cache[[]byte, []byte] {
	return s.db
}

// NewTransaction returns a new transaction against the store.
// Note that ristretto does not support the tx semantics.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return NewTx(s.db, s.ttl), nil
}

// Close closes the cache.
// Be sure to call this to release the goroutines.
func (s *Store) Close() {
	s.db.Close()
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
