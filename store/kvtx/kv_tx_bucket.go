package store_kvtx

import (
	"bytes"
	"regexp"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/mqueue"
	b58 "github.com/mr-tron/base58/base58"
)

// loadBucketConfig loads a bucket config at a key.
func (k *KVTx) loadBucketConfig(tx kvtx.Tx, key []byte) (*bucket.Config, error) {
	dat, found, err := tx.Get(key)
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
func (k *KVTx) ApplyBucketConfig(conf *bucket.Config) (
	updated bool,
	prev, curr *bucket.Config,
	err error,
) {
	dat, err := conf.MarshalVT()
	if err != nil {
		return false, nil, nil, err
	}

	key := k.kvkey.GetBucketConfigKey(conf.GetId())
	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return false, nil, nil, err
	}
	defer tx.Discard()

	// 1. lookup the existing config
	econf, err := k.loadBucketConfig(tx, key)
	if err != nil {
		return false, nil, nil, err
	}

	if econf != nil {
		if econf.GetVersion() >= conf.GetVersion() {
			return false, econf, econf, nil
		}
	}

	if err := tx.Set(key, dat); err != nil {
		return false, nil, nil, err
	}

	if err := tx.Commit(k.ctx); err != nil {
		return false, nil, nil, err
	}

	return true, econf, conf, nil
}

// GetBucketInfo returns bucket information by string.
func (k *KVTx) GetBucketInfo(id string) (*bucket.BucketInfo, error) {
	key := k.kvkey.GetBucketConfigKey(id)
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	bc, err := k.loadBucketConfig(tx, key)
	if err != nil {
		return nil, err
	}

	return bucket.NewBucketInfo(bc), nil
}

// ListBucketInfo lists buckets with an optional regex match.
func (k *KVTx) ListBucketInfo(idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error) {
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	resVals := make(map[string]int)
	var res []*bucket.BucketInfo
	prefix := k.kvkey.GetBucketConfigFullPrefix()
	err = tx.ScanPrefix(prefix, func(key, value []byte) error {
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
			if ev.GetConfig().GetVersion() >= bc.GetVersion() {
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
func (k *KVTx) GetBucketConfig(id string) (*bucket.Config, error) {
	key := k.kvkey.GetBucketConfigKey(id)
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	return k.loadBucketConfig(tx, key)
}

// GetReconcilerEventQueue returns a reference to the event queue for a
// reconciler ID. Should not return nil without an error.
func (k *KVTx) GetReconcilerEventQueue(pair bucket_store.BucketReconcilerPair) (mqueue.Queue, error) {
	prefix := k.kvkey.GetBucketMQueuePrefix()
	id := MarshalBucketReconcilerMqueueId(pair)
	prefixedID := bytes.Join([][]byte{prefix, id}, nil)
	return k.OpenMqueue(k.ctx, prefixedID)
}

// DeleteReconcilerEventQueue purges a reconciler event queue.
func (k *KVTx) DeleteReconcilerEventQueue(pair bucket_store.BucketReconcilerPair) error {
	prefix := k.kvkey.GetBucketMQueuePrefix()
	id := MarshalBucketReconcilerMqueueId(pair)
	prefixedID := bytes.Join([][]byte{prefix, id}, nil)
	return k.DelMqueue(k.ctx, prefixedID)
}

// ListFilledReconcilerEventQueues lists reconciler event queues that have
// at least one event, by reconciler ID.
func (k *KVTx) ListFilledReconcilerEventQueues() ([]bucket_store.BucketReconcilerPair, error) {
	prefix := k.kvkey.GetBucketMQueuePrefix()
	ids, err := k.ListMessageQueues(prefix, true)
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

// _ is a type assertion
var _ bucket_store.Store = ((*KVTx)(nil))
