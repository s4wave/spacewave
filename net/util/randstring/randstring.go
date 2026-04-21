package randstring

import (
	"crypto/rand"
	mrand "math/rand/v2"
	"strings"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

// RandString generates a random string with length n.
// Not cryptographically safe.
// Pass nil to use a random seed.
// https://stackoverflow.com/questions/22892120
func RandString(src *mrand.Rand, n int) string {
	if src == nil {
		var seed [32]byte
		_, _ = rand.Read(seed[:])
		src = mrand.New(mrand.NewChaCha8(seed)) //nolint:gosec
	}

	sb := strings.Builder{}
	sb.Grow(n)
	// A src.Int64() generates 64 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int64(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int64(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}
