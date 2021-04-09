package mock

import (
	"hash/crc64"
	"math/rand"
	"strings"
)

// BuildSeededRand builds a random seeded by string.
func BuildSeededRand(strs ...string) *rand.Rand {
	var sb strings.Builder
	for _, s := range strs {
		sb.WriteString(s)
	}
	seed := crc64.Checksum([]byte(sb.String()), crc64.MakeTable(crc64.ECMA))
	return rand.New(rand.NewSource(int64(seed)))
}
