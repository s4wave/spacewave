//go:build !tinygo || scheduler.tasks || scheduler.asyncify

package store

import "context"

func newPackReaderContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// startOwnerWork runs admitted index and range work independently.
func startOwnerWork(work func()) {
	go work()
}
