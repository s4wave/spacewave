package ui

import "time"

// FetchStatus is the loader UI's read-only view of launcher fetch progress.
type FetchStatus struct {
	// Fetching reports an in-flight launcher fetch.
	Fetching bool
	// HasConfig reports that a launcher configuration is available.
	HasConfig bool
	// LastErr is the latest fetch failure, or empty.
	LastErr string
	// Attempts counts fetch attempts.
	Attempts uint32
	// NextRetryAt is the next scheduled retry, or zero.
	NextRetryAt time.Time
}
