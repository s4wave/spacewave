package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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

	otx, err := a.objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open write transaction")
	}
	defer otx.Discard()

	if err := otx.Set(ctx, verifiedSOStateCacheKey(soID), data); err != nil {
		return errors.Wrap(err, "set verified SO cache")
	}
	if err := otx.Commit(ctx); err != nil {
		return err
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

	otx, err := a.objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open write transaction")
	}
	defer otx.Discard()

	if err := otx.Delete(ctx, verifiedSOStateCacheKey(soID)); err != nil {
		return errors.Wrap(err, "delete verified SO cache")
	}
	if err := otx.Commit(ctx); err != nil {
		return err
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

	otx, err := a.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open read transaction")
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, verifiedSOStateCacheKey(soID))
	if err != nil {
		return nil, errors.Wrap(err, "get verified SO cache")
	}
	if !found {
		return nil, nil
	}

	cache := &api.VerifiedSOStateCache{}
	if err := cache.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal verified SO cache")
	}
	return cache, nil
}
