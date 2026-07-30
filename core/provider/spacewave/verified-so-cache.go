package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx"
)

// verifiedSOStateCacheKeyPrefix is the ObjectStore key prefix for verified SO cache state.
const verifiedSOStateCacheKeyPrefix = "verified-so-state/"

// verifiedSOStateCacheKey returns the ObjectStore key for a verified SO cache entry.
func verifiedSOStateCacheKey(soID string) []byte {
	return []byte(verifiedSOStateCacheKeyPrefix + soID)
}

// writeVerifiedSOStateCache serializes verified SO cache state to the account ObjectStore.
func (a *ProviderAccount) writeVerifiedSOStateCache(
	ctx context.Context,
	soID string,
	cache *api.VerifiedSOStateCache,
) error {
	if a.objStore == nil {
		return errors.New("account object store not ready")
	}
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if cache == nil {
		return errors.New("verified SO cache is required")
	}

	data, err := cache.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal verified SO cache")
	}
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, verifiedSOStateCacheKey(soID), data); err != nil {
				return errors.Wrap(err, "set verified SO cache")
			}
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "write verified SO cache")
	}
	a.refreshSelfEnrollmentSummary(ctx)
	return nil
}

// deleteVerifiedSOStateCache removes the persisted verified SO cache entry
// for soID, if any. Missing entries are not an error: the caller is asking
// for a cold next-mount, which a missing entry already produces.
func (a *ProviderAccount) deleteVerifiedSOStateCache(
	ctx context.Context,
	soID string,
) error {
	if a.objStore == nil {
		return errors.New("account object store not ready")
	}
	if soID == "" {
		return errors.New("shared object id is required")
	}

	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Delete(ctx, verifiedSOStateCacheKey(soID)); err != nil {
				return errors.Wrap(err, "delete verified SO cache")
			}
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "delete verified SO cache")
	}
	a.refreshSelfEnrollmentSummary(ctx)
	return nil
}

// InvalidateVerifiedChain clears the persisted verified config-chain record
// for soID so the next mount re-verifies from scratch via /config-chain.
// Used by rejoin, recovery, and forced re-verification flows where the
// previously trusted chain head is no longer authoritative. The live
// cloudSOHost (if any) keeps its in-memory verified state for the
// remainder of its session; only future mounts observe the cold cache.
func (a *ProviderAccount) InvalidateVerifiedChain(ctx context.Context, soID string) error {
	return a.deleteVerifiedSOStateCache(ctx, soID)
}

// loadVerifiedSOStateCache reads verified SO cache state from the account ObjectStore.
func (a *ProviderAccount) loadVerifiedSOStateCache(
	ctx context.Context,
	soID string,
) (*api.VerifiedSOStateCache, error) {
	if a.objStore == nil {
		return nil, errors.New("account object store not ready")
	}
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}

	var cache *api.VerifiedSOStateCache
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, verifiedSOStateCacheKey(soID))
			if err != nil {
				return errors.Wrap(err, "get verified SO cache")
			}
			if !found {
				cache = nil
				return nil
			}
			next := &api.VerifiedSOStateCache{}
			if err := next.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal verified SO cache")
			}
			cache = next
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "open read transaction")
	}
	return cache, nil
}
