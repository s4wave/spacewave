package randstring

import (
	"crypto/rand"
	mrand "math/rand/v2"
	"strings"
)

// letterBytes is the uniformly sampled output alphabet.
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const (
	// letterIdxBits is the number of bits needed for an alphabet index.
	letterIdxBits = 6
	// letterIdxMask selects one candidate alphabet index.
	letterIdxMask = 1<<letterIdxBits - 1
	// letterIdxMax is the number of candidates in a nonnegative int64.
	letterIdxMax = 63 / letterIdxBits
)

// RandString generates n ASCII letters. A nil source uses crypto/rand directly;
// a supplied source preserves its deterministic sequence and is not secure.
// Failure to obtain system entropy panics. A negative length panics.
func RandString(src *mrand.Rand, n int) string {
	// Build the requested identifier from random alphabet indexes.
	sb := strings.Builder{}
	sb.Grow(n)

	// Sample fresh letters in batches, rejecting indexes outside the alphabet.
	if src == nil {
		var buf [32]byte
		for sb.Len() < n {
			if _, err := rand.Read(buf[:]); err != nil {
				panic(err)
			}
			for _, value := range buf {
				if idx := int(value & letterIdxMask); idx < len(letterBytes) {
					sb.WriteByte(letterBytes[idx])
					if sb.Len() == n {
						return sb.String()
					}
				}
			}
		}
		return sb.String()
	}

	// Consume seeded values in the established order for reproducible output.
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
