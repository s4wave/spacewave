// Package fatal carries process-fatal boot conditions from bus
// controllers to the entrypoint command that owns the process exit.
//
// A controller that discovers a condition under which the daemon
// cannot serve its purpose (for example, another live daemon already
// owns the front-door socket) reports it here instead of returning an
// error into the controllerbus retry loop, where it would be retried
// with backoff while the daemon keeps running without its front door.
// The blocking entrypoint command watches the channel and exits with
// the reported error.
package fatal

import "sync"

// broker is the process-wide fatal-condition record. The first
// reported error wins; later reports are dropped.
var broker = struct {
	mtx sync.Mutex
	err error
	ch  chan struct{}
}{ch: make(chan struct{})}

// Report records err as a process-fatal condition and wakes watchers.
// The first reported error is kept; a nil err or a later report is a
// no-op.
func Report(err error) {
	if err == nil {
		return
	}
	broker.mtx.Lock()
	defer broker.mtx.Unlock()
	if broker.err != nil {
		return
	}
	broker.err = err
	close(broker.ch)
}

// Chan returns a channel closed after the first Report.
func Chan() <-chan struct{} {
	return broker.ch
}

// Err returns the recorded fatal error, or nil before the first
// Report.
func Err() error {
	broker.mtx.Lock()
	defer broker.mtx.Unlock()
	return broker.err
}
