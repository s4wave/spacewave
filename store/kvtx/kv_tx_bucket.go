package kvtx

import (
	"errors"
	"regexp"
	"time"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/store/mqueue"
	"github.com/golang/protobuf/proto"
)

// loadBucketConfig loads a bucket config at a key.
func (k *KVTx) loadBucketConfig(tx Tx, key []byte) (*bucket.Config, error) {
	dat, found, err := tx.Get(key)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil
	}

	m := &bucket.Config{}
	if err := proto.Unmarshal(dat, m); err != nil {
		return nil, err
	}

	return m, nil
}

// PutBucketConfig puts a bucket configuration.
// Returns the previous and current (updated) configurations.
// The current configuration may be nil if the volume rejects the bucket.
// If outdated, prev == curr.
func (k *KVTx) PutBucketConfig(conf *bucket.Config) (
	updated bool,
	prev, curr *bucket.Config,
	err error,
) {
	dat, err := proto.Marshal(conf)
	if err != nil {
		return false, nil, nil, err
	}

	// use 0 for version, since we have a tx store, we can atomically replace
	// the configuration key
	key := k.kvkey.GetBucketConfigKey(conf.GetId(), 0)
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

	if err := tx.Set(key, dat, time.Duration(0)); err != nil {
		return false, nil, nil, err
	}

	if err := tx.Commit(k.ctx); err != nil {
		return false, nil, nil, err
	}

	return true, econf, conf, nil
}

// GetBucketInfo returns bucket information by string.
func (k *KVTx) GetBucketInfo(id string) (*bucket.BucketInfo, error) {
	key := k.kvkey.GetBucketConfigKey(id, 0)
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
	err = tx.ScanPrefix(prefix, func(key []byte) error {
		bc, err := k.loadBucketConfig(tx, key)
		if err != nil || bc == nil {
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

// GetLatestBucketConfig gets the bucket config with the highest revision.
// Can return nil if no bucket config is found.
func (k *KVTx) GetLatestBucketConfig(id string) (*bucket.Config, error) {
	key := k.kvkey.GetBucketConfigKey(id, 0)
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
	return k.getReconcilerEventQueue(pair)
}

// DeleteReconcilerEventQueue purges a reconciler event queue.
func (k *KVTx) DeleteReconcilerEventQueue(pair bucket_store.BucketReconcilerPair) error {
	mq, err := k.getReconcilerEventQueue(pair)
	if err != nil {
		return err
	}
	return mq.DeleteQueue()
}

// ListFilledReconcilerEventQueues lists reconciler event queues that have
// at least one event, by reconciler ID.
func (k *KVTx) ListFilledReconcilerEventQueues() ([]bucket_store.BucketReconcilerPair, error) {
	prefix := k.kvkey.GetBucketReconcilerMQueueMetaPrefix()
	return listFilledMQueues(k, prefix)
}

// getReconcilerEventQueue returns the mqueue for the pair.
func (k *KVTx) getReconcilerEventQueue(pair bucket_store.BucketReconcilerPair) (*mQueue, error) {
	if pair.ReconcilerID == "" || pair.BucketID == "" {
		return nil, errors.New("bucket/reconciler id is empty")
	}
	return newMQueue(k, pair.BucketID, pair.ReconcilerID), nil
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KVTx) PutBlock(ref *cid.BlockRef, data []byte) error {
	rm, err := ref.MarshalKey()
	if err != nil {
		return err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	if exists, _ := tx.Exists(key); exists {
		return nil
	}

	if err := tx.Set(key, data, time.Duration(0)); err != nil {
		return err
	}

	return tx.Commit(k.ctx)
}

// GetBlock looks up a block in the store.
// Returns data, found, and any exceptional error.
func (k *KVTx) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()

	return tx.Get(key)
}
