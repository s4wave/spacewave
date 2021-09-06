package kvtx_genji

import "bytes"

const (
	separator   byte = byte('/')
	storeKey         = "i"
	storePrefix      = 's'
	seqnumKey        = "seq"
)

// buildStoreKey builds the key with information about a store.
func buildStoreKey(name []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(storeKey) + 1 + len(name))
	buf.WriteString(storeKey)
	buf.WriteByte(separator)
	buf.Write(name)

	return buf.Bytes()
}

// buildStorePrefixKey builds the key prefix with data for the store.
func buildStorePrefixKey(name []byte) []byte {
	prefix := make([]byte, 0, len(name)+5)
	prefix = append(prefix, storePrefix)
	prefix = append(prefix, separator)
	prefix = append(prefix, name...)

	return prefix
}

// buildTransientKey builds the key with information about a transient engine.
/*
const (
	transientKey    = "ti"
	transientPrefix = "ts"
)
func buildTransientKey(name []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(transientKey) + 1 + len(name))
	buf.WriteString(transientKey)
	buf.WriteByte(separator)
	buf.Write(name)

	return buf.Bytes()
}

// buildTransientPrefixKey builds the key prefix with data for the transient engine.
func buildTransientPrefixKey(name []byte) []byte {
	prefix := make([]byte, 0, len(name)+len(transientPrefix)+1)
	prefix = append(prefix, []byte(transientPrefix)...)
	prefix = append(prefix, separator)
	prefix = append(prefix, name...)

	return prefix
}
*/
