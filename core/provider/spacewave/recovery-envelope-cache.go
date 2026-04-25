package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// recoveryEnvelopeCacheKeyPrefix is the ObjectStore key prefix for cached
// recovery envelopes.
const recoveryEnvelopeCacheKeyPrefix = "recovery-envelope/"

// recoveryEnvelopeCacheKey returns the ObjectStore key for the cached recovery
// envelope of soID.
func recoveryEnvelopeCacheKey(soID string) []byte {
	return []byte(recoveryEnvelopeCacheKeyPrefix + soID)
}

// loadRecoveryEnvelopeCache reads the cached recovery envelope for soID.
// Returns (nil, nil) when no entry exists or the account ObjectStore is not
// yet mounted (cold-start race or test harness without persistence).
func (a *ProviderAccount) loadRecoveryEnvelopeCache(
	ctx context.Context,
	soID string,
) (*sobject.SOEntityRecoveryEnvelope, error) {
	if a.objStore == nil {
		return nil, nil
	}
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}

	otx, err := a.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open read transaction")
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, recoveryEnvelopeCacheKey(soID))
	if err != nil {
		return nil, errors.Wrap(err, "get recovery envelope cache")
	}
	if !found {
		return nil, nil
	}

	env := &sobject.SOEntityRecoveryEnvelope{}
	if err := env.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal recovery envelope cache")
	}
	return env, nil
}

// writeRecoveryEnvelopeCache persists the recovery envelope for soID. When
// the account ObjectStore is not yet mounted the call is a no-op so callers
// can treat persistence as best-effort.
func (a *ProviderAccount) writeRecoveryEnvelopeCache(
	ctx context.Context,
	soID string,
	env *sobject.SOEntityRecoveryEnvelope,
) error {
	if a.objStore == nil {
		return nil
	}
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if env == nil {
		return errors.New("recovery envelope is required")
	}

	data, err := env.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal recovery envelope")
	}

	otx, err := a.objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open write transaction")
	}
	defer otx.Discard()

	if err := otx.Set(ctx, recoveryEnvelopeCacheKey(soID), data); err != nil {
		return errors.Wrap(err, "set recovery envelope cache")
	}
	return otx.Commit(ctx)
}

// deleteRecoveryEnvelopeCache removes the cached recovery envelope for soID.
// Missing entries (or a not-yet-mounted ObjectStore) are not an error.
func (a *ProviderAccount) deleteRecoveryEnvelopeCache(
	ctx context.Context,
	soID string,
) error {
	if a.objStore == nil {
		return nil
	}
	if soID == "" {
		return errors.New("shared object id is required")
	}

	otx, err := a.objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open write transaction")
	}
	defer otx.Discard()

	if err := otx.Delete(ctx, recoveryEnvelopeCacheKey(soID)); err != nil {
		return errors.Wrap(err, "delete recovery envelope cache")
	}
	return otx.Commit(ctx)
}
