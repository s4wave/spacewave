package prng

import (
	"bytes"
	"hash/crc64"
	"math/rand"
)

// BuildSeededRand builds a random seeded by string.
func BuildSeededRand(datas ...[]byte) *rand.Rand {
	var data bytes.Buffer
	for _, d := range datas {
		_, _ = data.Write(d)
	}
	seed := crc64.Checksum(data.Bytes(), crc64.MakeTable(crc64.ECMA))
	return rand.New(rand.NewSource(int64(seed)))
}
