package manifest

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// IndexCache is a kvtx-backed cache for raw kvfile index-tail bytes.
type IndexCache struct {
	// store persists raw index tails by pack ID.
	store kvtx.Store
}

// NewIndexCache creates a new IndexCache backed by the given store.
func NewIndexCache(store kvtx.Store) *IndexCache {
	return &IndexCache{store: store}
}

// Get returns cached raw index-tail bytes for a packfile.
func (c *IndexCache) Get(ctx context.Context, packID string) ([]byte, bool, error) {
	// Read the cached tail in one transaction.
	var data []byte
	var found bool
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return c.store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Read the entry, copying it out of the transaction.
			value, attemptFound, err := tx.Get(ctx, indexCacheKey(packID))
			if err != nil {
				return errors.Wrap(err, "get index cache entry")
			}
			data, found = nil, attemptFound
			if attemptFound {
				data = bytes.Clone(value)
			}
			return nil
		},
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "open index cache transaction")
	}
	return data, found, nil
}

// Set stores raw index-tail bytes for a packfile.
func (c *IndexCache) Set(ctx context.Context, packID string, data []byte) error {
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return c.store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, indexCacheKey(packID), bytes.Clone(data)); err != nil {
				return errors.Wrap(err, "set index cache entry")
			}
			return nil
		},
	)
}
