package store_kvkey

import (
	"bytes"
	b58 "github.com/mr-tron/base58/base58"
	"strconv"
)

// KVKey is the key/value key generator.
type KVKey struct {
	conf Config
}

// NewKVKey builds a new KV key generator from a config.
// Can pass nil to use default.
func NewKVKey(conf *Config) (*KVKey, error) {
	if conf == nil {
		conf = DefaultConfig()
	} else {
		if err := conf.Validate(); err != nil {
			return nil, err
		}
	}

	return &KVKey{conf: *conf}, nil
}

// GetBlockFullPrefix returns the prefix for all blocks.
func (k *KVKey) GetBlockFullPrefix() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBlockPrefix(),
	}, nil)
}

// GetBlockKey returns the key for the given block.
func (k *KVKey) GetBlockKey(refMarshalKey []byte) []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBlockPrefix(),
		[]byte(b58.FastBase58Encoding(refMarshalKey)),
	}, nil)
}

// GetBucketConfigFullPrefix returns the prefix for all bucket configs.
func (k *KVKey) GetBucketConfigFullPrefix() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketConfigPrefix(),
	}, nil)
}

// GetBucketConfigKey returns the key for the given id and rev.
func (k *KVKey) GetBucketConfigKey(id string, rev uint32) []byte {
	revStr := strconv.FormatUint(uint64(rev), 10)
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketConfigPrefix(),
		[]byte(id),
		[]byte("-"),
		[]byte(revStr),
	}, nil)
}

// GetBucketReconcilerMQueuePrefix returns the bucket reconciler message queue prefix.
func (k *KVKey) GetBucketReconcilerMQueuePrefix(bucketID, reconcilerID string) []byte {
	if bucketID == "" && reconcilerID == "" {
		return bytes.Join([][]byte{
			k.conf.GetPrefix(),
			k.conf.GetBucketReconcilerMqueuePrefix(),
		}, nil)
	}

	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketReconcilerMqueuePrefix(),
		[]byte(bucketID),
		[]byte("-"),
		[]byte(reconcilerID),
		[]byte("/"),
	}, nil)
}

// GetBucketReconcilerMQueueMetaPrefix returns the bucket reconciler message queue metadata prefix.
func (k *KVKey) GetBucketReconcilerMQueueMetaPrefix() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketReconcilerMqueuePrefix(),
		[]byte("meta/"),
	}, nil)
}

// GetBucketReconcilerMQueueMetaKey returns the bucket reconciler message queue metadata key.
func (k *KVKey) GetBucketReconcilerMQueueMetaKey(bucketID, reconcilerID string) []byte {
	return bytes.Join([][]byte{
		k.GetBucketReconcilerMQueueMetaPrefix(),
		[]byte(bucketID),
		[]byte("-"),
		[]byte(reconcilerID),
	}, nil)
}

// GetPeerPrivKey returns the key to use for the peer private key.
func (k *KVKey) GetPeerPrivKey() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetPeerPrivKey(),
	}, nil)
}

// GetObjectStorePrefixByID returns the prefix to use for an object store with an id.
func (k *KVKey) GetObjectStorePrefixByID(objStoreID string) []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetObjectStorePrefix(),
		[]byte(objStoreID),
		[]byte("/"),
	}, nil)
}
