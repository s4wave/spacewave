package store_kvtx

import (
	"context"
	"regexp"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	"github.com/s4wave/spacewave/db/kvtx"
)

// loadBucketConfig loads a bucket config at a key.
func (k *KVTx) loadBucketConfig(ctx context.Context, tx kvtx.Tx, key []byte) (*bucket.Config, error) {
	// Read the bucket configuration bytes from the transaction.
	dat, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Leave an absent bucket configuration unset.
	if !found {
		return nil, nil
	}

	// Decode the stored bucket configuration.
	m := &bucket.Config{}
	if err := m.UnmarshalVT(dat); err != nil {
		return nil, err
	}

	return m, nil
}

// ApplyBucketConfig applies a bucket configuration.
// Returns the previous and current (updated) configurations.
// The current configuration may be nil if the volume rejects the bucket.
// If outdated, prev == curr. Invalid storage snapshots retry the complete update.
func (k *KVTx) ApplyBucketConfig(ctx context.Context, conf *bucket.Config) (
	updated bool,
	prev, curr *bucket.Config,
	err error,
) {
	// Validate the bucket configuration before storing it.
	if err := conf.Validate(); err != nil {
		return false, nil, nil, err
	}

	// Encode the bucket configuration for the transaction.
	dat, err := conf.MarshalVT()
	if err != nil {
		return false, nil, nil, err
	}

	// Apply the bucket revision in a retryable write transaction.
	key := k.kvkey.GetBucketConfigKey(conf.GetId())
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return k.store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Read the stored bucket revision afresh for this transaction attempt.
			updated, prev, curr = false, nil, nil
			existing, err := k.loadBucketConfig(ctx, tx, key)
			if err != nil {
				return err
			}
			if existing != nil && existing.GetRev() >= conf.GetRev() {
				prev, curr = existing, existing
				return nil
			}

			// Store the newer bucket configuration and report the replacement.
			if err := tx.Set(ctx, key, dat); err != nil {
				return err
			}
			updated, prev, curr = true, existing, conf
			return nil
		})
	if err != nil {
		return false, nil, nil, err
	}

	return updated, prev, curr, nil
}

// GetBucketInfo returns bucket information by string.
func (k *KVTx) GetBucketInfo(ctx context.Context, id string) (*bucket.BucketInfo, error) {
	// Open a read transaction for the requested bucket configuration.
	key := k.kvkey.GetBucketConfigKey(id)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Load the bucket configuration used by the information record.
	bc, err := k.loadBucketConfig(ctx, tx, key)
	if err != nil {
		return nil, err
	}

	return bucket.NewBucketInfo(bc), nil
}

// ListBucketInfo lists buckets with an optional regex match.
func (k *KVTx) ListBucketInfo(ctx context.Context, idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error) {
	// Open a read transaction over the stored bucket configurations.
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Collect matching bucket information from the configuration prefix.
	resVals := make(map[string]int)
	var res []*bucket.BucketInfo
	prefix := k.kvkey.GetBucketConfigFullPrefix()
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Decode the bucket configuration at this scan position.
		bc := &bucket.Config{}
		if err := bc.UnmarshalVT(value); err != nil {
			return err
		}

		// Exclude bucket IDs outside the requested pattern.
		if idRegex != nil {
			if !idRegex.MatchString(bc.GetId()) {
				return nil
			}
		}

		// Replace a collected bucket record only with a newer revision.
		nbi := bucket.NewBucketInfo(bc)
		if evi, ok := resVals[bc.GetId()]; ok {
			ev := res[evi]
			if ev.GetConfig().GetRev() >= bc.GetRev() {
				return nil
			}
			res[evi] = nbi
			return nil
		}

		// Include the bucket information in the scan results.
		res = append(res, nbi)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

// GetBucketConfig gets the bucket config for the bucket ID.
// Can return nil if no bucket config is found.
func (k *KVTx) GetBucketConfig(ctx context.Context, id string) (*bucket.Config, error) {
	// Open a read transaction for the requested bucket configuration.
	key := k.kvkey.GetBucketConfigKey(id)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	return k.loadBucketConfig(ctx, tx, key)
}

// _ is a type assertion
var _ bucket_store.Store = (*KVTx)(nil)
