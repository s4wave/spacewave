//go:build !js

package spacewave_cli

import (
	"os"
	"runtime/debug"

	"github.com/s4wave/spacewave/bldr/util/hostmem"
	"github.com/sirupsen/logrus"
)

// serveMemoryShare is the divisor of physical memory that sets the daemon's
// default soft memory limit.
const serveMemoryShare = 4

// minServeMemoryLimit is the smallest default soft memory limit in bytes.
const minServeMemoryLimit = 4 << 30

// setServeMemoryLimit sets the daemon's soft memory limit to a quarter of
// physical memory, and at least minServeMemoryLimit, unless GOMEMLIMIT sets
// one. Without a limit the collector lets the heap grow to twice its live
// size, which the page caches and checkpoints of a large Volume make
// gigabytes. The limit must stay well above the live heap, about 2 GB for a
// busy daemon: with the limit near the live heap the collector runs
// continuously, up to the runtime's GC CPU limit. No allocation fails past
// the limit.
func setServeMemoryLimit(le *logrus.Entry) {
	// Keep a limit the operator set.
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}

	// Derive the limit from physical memory.
	total, err := hostmem.Total()
	if err != nil {
		le.WithError(err).Warn("leaving the memory limit unset")
		return
	}
	limit := max(total/serveMemoryShare, minServeMemoryLimit)
	debug.SetMemoryLimit(int64(limit)) // #nosec G115 -- a quarter of a uint64 fits in int64.
	le.WithField("memory-limit-bytes", limit).Info("set the soft memory limit")
}
