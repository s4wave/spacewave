//go:build !js && !wasip1

package store_kvtx_bolt

import "sync/atomic"

// BatchStats tracks batch store performance.
type BatchStats struct {
	// Writes counts Set calls accepted by batch transactions.
	Writes atomic.Int64
	// Commits counts underlying Bolt commit attempts.
	Commits atomic.Int64
}
