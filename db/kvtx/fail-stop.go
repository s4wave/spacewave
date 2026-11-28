package kvtx

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
)

// FailStopEnv names the switch for fail-stop in the stores. Set to "off", a
// store's failed flush fails only its own call: a later flush that succeeds
// marks every commit before it durable, a cancelled durable commit returns at
// once, and the store keeps serving transactions, as before fail-stop.
const FailStopEnv = "SPACEWAVE_STORE_FAIL_STOP"

// FailStopOff turns fail-stop off for every store opened in this process.
// It is read from FailStopEnv at start, and tests set it before opening.
var FailStopOff = os.Getenv(FailStopEnv) == "off"

// ErrFlushFailed is returned, joined with the cause, by every flush, commit
// and new transaction of a store after one of its durable flushes failed.
// The store's outcome for the commits since its last durable flush is
// unknown, not absent: close the store and open it again, and recovery shows
// what the file holds.
var ErrFlushFailed = errors.New("an earlier flush of the store failed")

// FailStop is the fail-stop state of one stored file, shared by every
// OrderedFlusher that flushes it, such as the flushers of several stores on
// one SQLite file in a process. A failure belongs to the file, not to the
// store whose flush saw it: Linux reports a writeback error to one fdatasync
// of an open file, so a flush by another store could succeed without the lost
// pages, and that store's readers would take the lost commits as stored.
//
// Invariant: once Fail records a failure, every flusher sharing the FailStop
// refuses, and none of their flushes or admitted calls is still running.
// Flushes of the file run one at a time under flushMtx, each recording its
// failure before the next starts, and Fail waits under the gate for every
// call the flushers admitted.
type FailStop struct {
	// on records failures; it is off when FailStopOff was set at creation.
	on bool
	// flushMtx serializes the file's flushes and the calls Flushing runs,
	// each checking for the failure and recording its own under it.
	flushMtx sync.Mutex
	// gate orders each call Admit runs against the failure: Admit holds it
	// shared from its check through the call's end, and Fail holds it alone.
	gate sync.RWMutex
	// failed is the first failure, joined with ErrFlushFailed, or nil.
	failed atomic.Pointer[error]
	// done is closed when failed is set.
	done chan struct{}
}

// NewFailStop returns the fail-stop state of a file with no failure, which
// records none when FailStopOff is set.
func NewFailStop() *FailStop {
	return &FailStop{on: !FailStopOff, done: make(chan struct{})}
}

// Failed returns the first failure, joined with ErrFlushFailed, or nil.
func (s *FailStop) Failed() error {
	if p := s.failed.Load(); p != nil {
		return *p
	}
	return nil
}

// Done returns a channel closed once a failure is recorded.
func (s *FailStop) Done() <-chan struct{} {
	return s.done
}

// Fail records err as the file's failure, unless fail-stop is off or a
// failure is recorded, and returns the recorded failure. It waits for every
// call Admit is running, so it must not be called from one.
func (s *FailStop) Fail(err error) error {
	// With fail-stop off, pass the error through unrecorded.
	if !s.on {
		return err
	}

	// Record the failure once no admitted call runs, then wake the waiters.
	joined := errors.Join(ErrFlushFailed, err)
	s.gate.Lock()
	if s.failed.CompareAndSwap(nil, &joined) {
		close(s.done)
	}
	s.gate.Unlock()
	return s.Failed()
}

// Admit runs call unless the file failed, and returns the failure instead. A
// failure is recorded only once no admitted call is in progress, so each call
// either ends before the failure or does not start. call must not call Admit,
// Fail or Flushing: Fail waits for call, and a failing flush holds flushMtx
// while it waits.
func (s *FailStop) Admit(call func() error) error {
	s.gate.RLock()
	defer s.gate.RUnlock()
	if err := s.Failed(); err != nil {
		return err
	}
	return call()
}

// Flushing runs call, a flush of the file or a write that flushes it, after
// every earlier flush has ended and recorded its failure, unless the file
// failed, and returns the failure instead. It records an error of call for
// which storageErr reports true as the file's failure, and returns call's
// other errors as they are; a nil storageErr counts every error. call must not
// call Admit, Fail or Flushing, nor a flusher's Sync or Close.
func (s *FailStop) Flushing(call func() error, storageErr func(error) bool) error {
	// Run call alone among the file's flushes unless one failed, and record
	// its storage failure before the next flush starts.
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	if err := s.Failed(); err != nil {
		return err
	}
	err := call()
	if err != nil && (storageErr == nil || storageErr(err)) {
		return s.Fail(err)
	}
	return err
}
