//go:build js && wasm

package store

import (
	"github.com/mr-tron/base58/base58"
)

// encodeKey encodes a binary key to a base58 filename.
func encodeKey(key []byte) string {
	return base58.Encode(key)
}

// decodeKey decodes a base58 filename back to a binary key.
func decodeKey(encoded string) ([]byte, error) {
	return base58.Decode(encoded)
}

// shardPrefix returns the first character of a base58-encoded key,
// used as the shard directory name.
func shardPrefix(encoded string) string {
	if len(encoded) == 0 {
		return "_"
	}
	return encoded[:1]
}
