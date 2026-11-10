//go:build !unix || js || wasip1

package packahead

import "math"

// freeDiskBytes reports unbounded space where the platform cannot measure it,
// so a reserve refuses nothing.
func freeDiskBytes(string) (uint64, error) {
	return math.MaxUint64, nil
}
