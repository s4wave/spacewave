package store_kvtx

import (
	"bytes"
	"context"
	"regexp"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/mqueue"
)

// MarshalBucketReconcilerMqueueId encodes an id.
func MarshalBucketReconcilerMqueueId(pair bucket_store.BucketReconcilerPair) []byte {
	d, _ := (&BucketReconcilerMqueueId{
		BucketId:     pair.BucketID,
		ReconcilerId: pair.ReconcilerID,
	}).MarshalVT()
	return []byte(b58.FastBase58Encoding(d))
}

// UnmarshalBucketReconcilerMqueueId decodes an id.
//
// If parse error returns empty values.
func UnmarshalBucketReconcilerMqueueId(dat []byte) bucket_store.BucketReconcilerPair {
	b := bucket_store.BucketReconcilerPair{}
	p, err := b58.Decode(string(dat))
	if err != nil {
		return b
	}
	brmi := &BucketReconcilerMqueueId{}
	if err = brmi.UnmarshalVT(p); err != nil {
		return b
	}
	b.BucketID = brmi.GetBucketId()
	b.ReconcilerID = brmi.GetReconcilerId()
	return b
}

// loadBucketConfig loads a bucket config at a key.
func (k *KVTx) loadBucketConfig(ctx context.Context, tx kvtx.Tx, key []byte) (*bucket.Config, error) {
	dat, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil
	}

	m := &bucket.Config{}
	if err := m.UnmarshalVT(dat); err != nil {
		return nil, err
	}

	return m, nil
}

// ApplyBucketConfig applies a bucket configuration.
// Returns the previous and current (updated) configurations.
// The current configuration may be nil if the volume rejects the bucket.
// If outdated, prev == curr.
func (k *KVTx) ApplyBucketConfig(ctx context.Context, conf *bucket.Config) (
	updated bool,
	prev, curr *bucket.Config,
	err error,
) {
	if err := conf.Validate(); err != nil {
		return false, nil, nil, err
	}

	dat, err := conf.MarshalVT()
	if err != nil {
		return false, nil, nil, err
	}

	key := k.kvkey.GetBucketConfigKey(conf.GetId())
	tx, err := k.store.NewTransaction(ctx, true)
	if err != nil {
		return false, nil, nil, err
	}
	defer tx.Discard()

	// 1. lookup the existing config
	econf, err := k.loadBucketConfig(ctx, tx, key)
	if err != nil {
		return false, nil, nil, err
	}

	if econf != nil {
		if econf.GetRev() >= conf.GetRev() {
			return false, econf, econf, nil
		}
	}

	if err := tx.Set(ctx, key, dat); err != nil {
		return false, nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, nil, nil, err
	}

	return true, econf, conf, nil
}

// GetBucketInfo returns bucket information by string.
func (k *KVTx) GetBucketInfo(ctx context.Context, id string) (*bucket.BucketInfo, error) {
	key := k.kvkey.GetBucketConfigKey(id)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	bc, err := k.loadBucketConfig(ctx, tx, key)
	if err != nil {
		return nil, err
	}

	return bucket.NewBucketInfo(bc), nil
}

// ListBucketInfo lists buckets with an optional regex match.
func (k *KVTx) ListBucketInfo(ctx context.Context, idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error) {
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	resVals := make(map[string]int)
	var res []*bucket.BucketInfo
	prefix := k.kvkey.GetBucketConfigFullPrefix()
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		bc := &bucket.Config{}
		if err := bc.UnmarshalVT(value); err != nil {
			return err
		}

		if idRegex != nil {
			if !idRegex.MatchString(bc.GetId()) {
				return nil
			}
		}

		nbi := bucket.NewBucketInfo(bc)
		if evi, ok := resVals[bc.GetId()]; ok {
			ev := res[evi]
			if ev.GetConfig().GetRev() >= bc.GetRev() {
				return nil
			}
			res[evi] = nbi
			return nil
		}

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
	key := k.kvkey.GetBucketConfigKey(id)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	return k.loadBucketConfig(ctx, tx, key)
}

// GetReconcilerEventQueue returns a reference to the event queue for a
// reconciler ID. Should not return nil without an error.
func (k *KVTx) GetReconcilerEventQueue(ctx context.Context, pair bucket_store.BucketReconcilerPair) (mqueue.Queue, error) {
	prefix := k.kvkey.GetBucketMQueuePrefix()
	id := MarshalBucketReconcilerMqueueId(pair)
	prefixedID := bytes.Join([][]byte{prefix, id}, nil)
	return k.OpenMqueue(ctx, prefixedID)
}

// DeleteReconcilerEventQueue purges a reconciler event queue.
func (k *KVTx) DeleteReconcilerEventQueue(ctx context.Context, pair bucket_store.BucketReconcilerPair) error {
	prefix := k.kvkey.GetBucketMQueuePrefix()
	id := MarshalBucketReconcilerMqueueId(pair)
	prefixedID := bytes.Join([][]byte{prefix, id}, nil)
	return k.DelMqueue(ctx, prefixedID)
}

// ListFilledReconcilerEventQueues lists reconciler event queues that have
// at least one event, by reconciler ID.
func (k *KVTx) ListFilledReconcilerEventQueues(ctx context.Context) ([]bucket_store.BucketReconcilerPair, error) {
	prefix := k.kvkey.GetBucketMQueuePrefix()
	ids, err := k.ListMessageQueues(ctx, prefix, true)
	if err != nil {
		return nil, err
	}
	res := make([]bucket_store.BucketReconcilerPair, 0, len(ids))
	for _, id := range ids {
		bp := UnmarshalBucketReconcilerMqueueId(id[len(prefix):])
		if bp.BucketID == "" || bp.ReconcilerID == "" {
			continue
		}
		res = append(res, bp)
	}
	return res, nil
}

// _ is a type assertion
var _ bucket_store.Store = ((*KVTx)(nil))
