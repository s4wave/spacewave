package block_store

// PauseWatcher is a store whose requests can pause behind its service, as a
// bucket that answers with a throttle pauses every request to it until the
// throttle passes. A request made during a pause waits for it to end and does
// not fail, so a caller with another source for a block watches the pause to
// ask that source meanwhile, instead of waiting for a failure that never comes.
type PauseWatcher interface {
	// WatchPause reports whether the store's requests are paused, and returns
	// a channel closed at the next change, or nil when nothing can change.
	WatchPause() (paused bool, changed <-chan struct{})
}
