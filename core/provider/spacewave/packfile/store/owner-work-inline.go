//go:build tinygo && scheduler.none

package store

import "context"

func newPackReaderContext() (context.Context, context.CancelFunc) {
	return context.Background(), func() {}
}

// startOwnerWork runs admitted index and range work inline when the runtime has no scheduler.
func startOwnerWork(work func()) {
	work()
}
