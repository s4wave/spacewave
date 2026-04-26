//go:build !tinygo

package store

import (
	"runtime"

	"github.com/aperturerobotics/util/conc"
)

// defaultVerifyConcurrency returns the default verify/persist worker count.
func defaultVerifyConcurrency() int {
	n := runtime.GOMAXPROCS(0)
	if n <= 0 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

// newDefaultVerifyExecutor returns the production verify queue, backed by
// conc.ConcurrentQueue.
func newDefaultVerifyExecutor(maxConcurrency int) verifyExecutor {
	return conc.NewConcurrentQueue(maxConcurrency)
}
