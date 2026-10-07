package store_kvkey

import (
	"bytes"
	"strconv"
)

// FormatVersion is the version of the key layout KVKey builds.
//
// Version 0 keyed each block by the base58 text of its marshaled ref.
// Version 1 keys each block by the marshaled ref bytes.
const FormatVersion = 1

// formatVersionKey names the key holding the store's format version.
var formatVersionKey = []byte("format")

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
		refMarshalKey,
	}, nil)
}

// GetFormatVersionKey returns the key holding the store's format version.
func (k *KVKey) GetFormatVersionKey() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		formatVersionKey,
	}, nil)
}

// MarshalFormatVersion encodes a format version as the value of the format
// version key.
func MarshalFormatVersion(version uint64) []byte {
	return strconv.AppendUint(nil, version, 10)
}

// ParseFormatVersion decodes the value of the format version key.
func ParseFormatVersion(data []byte) (uint64, error) {
	return strconv.ParseUint(string(data), 10, 64)
}

// GetBucketConfigFullPrefix returns the prefix for all bucket configs.
func (k *KVKey) GetBucketConfigFullPrefix() []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketConfigPrefix(),
	}, nil)
}

// GetBucketConfigKey returns the key for the given id.
func (k *KVKey) GetBucketConfigKey(id string) []byte {
	return bytes.Join([][]byte{
		k.conf.GetPrefix(),
		k.conf.GetBucketConfigPrefix(),
		[]byte(id),
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
