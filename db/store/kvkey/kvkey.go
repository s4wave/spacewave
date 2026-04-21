package store_kvkey

import (
	"bytes"

	b58 "github.com/mr-tron/base58/base58"
)

// KVKey is the key/value key generator.
type KVKey struct {
	conf *Config
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

	return &KVKey{conf: conf}, nil
}

// NewDefaultKVKey constructs a KVKey with a default config.
func NewDefaultKVKey() *KVKey {
	return &KVKey{conf: DefaultConfig()}
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

// GetBucketConfigKey returns the key for the given id
func (k *KVKey) GetBucketConfigKey(id string) []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketConfigPrefix(),
		[]byte(id),
	}, nil)
}

// GetMqueuePrefix returns the prefix for the mqueue with id.
func (k *KVKey) GetMQueuePrefix(id []byte) []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetMqueuePrefix(),
		id,
		[]byte("/"),
	}, nil)
}

// GetMQueueMetaPrefix returns the bucket reconciler message queue metadata prefix.
func (k *KVKey) GetMQueueMetaPrefix() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetMqueueMetaPrefix(),
	}, nil)
}

// GetBucketReconcilerMQueueMetaKey returns the bucket reconciler message queue metadata key.
func (k *KVKey) GetBucketReconcilerMQueueMetaKey(id []byte) []byte {
	return bytes.Join([][]byte{
		k.GetMQueueMetaPrefix(),
		id,
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

// GetBucketMQueuePrefix returns the bucket reconciler message queue id prefix.
func (k *KVKey) GetBucketMQueuePrefix() []byte {
	return k.conf.GetBucketMqueuePrefix()
}
