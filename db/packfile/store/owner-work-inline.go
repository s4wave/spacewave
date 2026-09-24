//go:build tinygo && scheduler.none

package store

import "context"

// newPackReaderContext returns an uncancellable lifetime because inline work
// completes before its caller returns.
func newPackReaderContext() (context.Context, context.CancelFunc) {
	return context.Background(), func() {}
}

// startOwnerWork runs admitted index and range work inline when the runtime has no scheduler.
func startOwnerWork(work func()) {
	work()
}
