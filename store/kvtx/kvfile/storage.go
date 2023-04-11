package store_kvtx_kvfile

import "github.com/cespare/xxhash"

// keyType is the type used for keys.
type keyType uint64

// valType is the type used for values
type valType struct {
	key []byte
	val []byte
}

// hashKey hashes the key.
func hashKey(key []byte) uint64 {
	return xxhash.Sum64(key)
}
