package store_kvkey

import (
	"bytes"
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
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketReconcilerMqueuePrefix(),
		[]byte(bucketID),
		[]byte("-"),
		[]byte(reconcilerID),
		[]byte("/"),
	}, nil)
}

// GetPeerPrivKey returns the key to use for the peer private key.
func (k *KVKey) GetPeerPrivKey() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetPeerPrivKey(),
	}, nil)
}

// GetMessageQueuePrefix returnst h
