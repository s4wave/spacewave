package kvtx

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/golang/protobuf/proto"
	"time"
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
// If outdated, return false, nil
func (k *KVTx) PutBucketConfig(conf *bucket.Config) (outdated bool, err error) {
	dat, err := proto.Marshal(conf)
	if err != nil {
		return false, err
	}

	// use 0 for version, since we have a tx store, we can atomically replace
	// the configuration key
	key := k.kvkey.GetBucketConfigKey(conf.GetId(), 0)
	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	// 1. lookup the existing config
	econf, err := k.loadBucketConfig(tx, key)
	if err != nil {
		return false, err
	}

	if econf != nil {
		if econf.GetVersion() > conf.GetVersion() {
			return true, nil
		}
	}

	if err := tx.Set(key, dat, time.Duration(0)); err != nil {
		return false, err
	}

	return false, tx.Commit(k.ctx)
}

// GetLatestBucketConfig gets the bucket config with the highest revision.
// Can return nil if no bucket config is found.
func (k *KVTx) GetLatestBucketConfig(id []byte) (*bucket.Config, error) {
	key := k.kvkey.GetBucketConfigKey(id, 0)
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	return k.loadBucketConfig(tx, key)
}
