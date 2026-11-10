package kvtx

import (
	"context"
	"strings"
	"time"
)

// IsBusy reports whether err is a store's report that another writer holds its
// lock, such as SQLite's "database is locked" or SQLITE_BUSY, which a later
// attempt of the same transaction may pass.
func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}

// busyRetries bounds how often RetryBusy runs a transaction again after the
// store reported it busy, and busyWait is its first wait, doubling to
// busyMaxWait: about 80 seconds in all.
const (
	busyRetries = 20
	busyWait    = 250 * time.Millisecond
	busyMaxWait = 5 * time.Second
)

// RetryBusy runs fn, a transaction that commits whole or not at all, and runs
// it again after a backoff while the store reports itself busy, as IsBusy
// describes, up to busyRetries times. Each attempt is its own transaction, so a
// retry holds the writer no longer than the first attempt did. Returns fn's
// last error, or the context's cause if it ends during a wait.
func RetryBusy(ctx context.Context, fn func() error) error {
	wait := busyWait
	for busy := 0; ; busy++ {
		// Run the transaction, returning unless the store was busy.
		err := fn()
		if err == nil || busy >= busyRetries || !IsBusy(err) {
			return err
		}

		// Back off before the next attempt.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		//gostyle:timer backoff
		case <-time.After(wait):
		}
		wait = min(2*wait, busyMaxWait)
	}
}
