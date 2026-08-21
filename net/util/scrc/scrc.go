package scrc

import (
	"hash"
	"hash/crc64"
	"sync"
)

var (
	// h64Mtx guards h64, which is reused across Crc64 calls.
	h64Mtx sync.Mutex
	// h64 is the shared crc64 hash reused across Crc64 calls.
	h64 hash.Hash64
)

func init() {
	h64 = crc64.New(crc64.MakeTable(crc64.ECMA))
}

// Crc64 computes the crc64 of some data.
func Crc64(ds ...[]byte) uint64 {
	h64Mtx.Lock()
	defer h64Mtx.Unlock()
	defer h64.Reset()

	for _, d := range ds {
		_, _ = h64.Write(d)
	}

	return h64.Sum64()
}
