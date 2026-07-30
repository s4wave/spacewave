package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx"
)

// recoveryEntityKeypairsCacheKeyPrefix is the ObjectStore key prefix for
// cached per-entity recovery keypair sets. One fetch of a SO's
// /recovery-entity-keypairs response decomposes into one cache entry per
// readable participant entity, so the next SO that shares those entities
// reads from cache instead of issuing a fresh GET.
const recoveryEntityKeypairsCacheKeyPrefix = "recovery-entity-keypairs/"

// recoveryEntityKeypairsCacheKey returns the ObjectStore key for the cached
// keypair set of entityID.
func recoveryEntityKeypairsCacheKey(entityID string) []byte {
	return []byte(recoveryEntityKeypairsCacheKeyPrefix + entityID)
}

// loadRecoveryEntityKeypairsCache reads the cached keypair set for entityID.
// Returns (nil, nil) when no entry exists or the account ObjectStore is not
// yet mounted (cold-start race or test harness without persistence).
func (a *ProviderAccount) loadRecoveryEntityKeypairsCache(
	ctx context.Context,
	entityID string,
) (*api.SORecoveryEntityKeypairs, error) {
	if a.objStore == nil {
		return nil, nil
	}
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}

	var entry *api.SORecoveryEntityKeypairs
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, recoveryEntityKeypairsCacheKey(entityID))
			if err != nil {
				return errors.Wrap(err, "get entity keypairs cache")
			}
			if !found {
				entry = nil
				return nil
			}
			next := &api.SORecoveryEntityKeypairs{}
			if err := next.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal entity keypairs cache")
			}
			entry = next
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "open read transaction")
	}
	return entry, nil
}

// writeRecoveryEntityKeypairsCache persists entry into the cache for
// entry.EntityId. When the account ObjectStore is not yet mounted the call is a
// no-op so callers can treat persistence as best-effort.
func (a *ProviderAccount) writeRecoveryEntityKeypairsCache(
	ctx context.Context,
	entry *api.SORecoveryEntityKeypairs,
) error {
	if a.objStore == nil {
		return nil
	}
	if entry == nil || entry.GetEntityId() == "" {
		return errors.New("entity keypairs entry with entity id is required")
	}

	data, err := entry.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal entity keypairs")
	}
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, recoveryEntityKeypairsCacheKey(entry.GetEntityId()), data); err != nil {
				return errors.Wrap(err, "set entity keypairs cache")
			}
			return nil
		},
	)
	return errors.Wrap(err, "write entity keypairs cache")
}
