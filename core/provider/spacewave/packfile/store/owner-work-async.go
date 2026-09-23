//go:build !tinygo || !scheduler.none

package store

import "context"

// newPackReaderContext returns the cancellable lifetime of pack reader work.
func newPackReaderContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// startOwnerWork runs admitted index and range work independently.
func startOwnerWork(work func()) {
	go work()
}
