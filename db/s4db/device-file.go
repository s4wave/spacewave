package s4db

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/volume/device"
)

// deviceFile is a database file on a device. A device has one opener, so
// no other process shares the file: every lock is free and no watcher
// fires. A crash may keep or tear each unflushed device write on its own, so
// a write reaches the device in order but survives in no particular order.
type deviceFile struct {
	*device.Handle
}

// OpenDevice opens or creates the database in the named file of dev. ctx
// bounds every device call. The caller opens each file once.
func OpenDevice(ctx context.Context, dev device.Device, name string, opts Options) (*DB, error) {
	h, err := device.OpenHandle(ctx, dev, name)
	if err != nil {
		return nil, err
	}
	return open(deviceFile{h}, opts)
}

// size returns the file length.
func (f deviceFile) size() (int64, error) {
	return f.Size()
}

// truncate sets the file length.
func (f deviceFile) truncate(n int64) error {
	return f.Truncate(n)
}

// flushDurable flushes the earlier device calls, then issues the collected
// writes with a flush. Other files on the device, written before a commit,
// are then durable before any record of the commit, so a crash never keeps
// the commit without them.
func (f deviceFile) flushDurable() error {
	if err := f.Barrier(); err != nil {
		return err
	}
	return f.Sync()
}

// flushOrdered issues the collected writes without a flush, so a later flush
// through any handle on the device makes them durable. A crash may keep any
// subset of them, so ordered commits rely on record checksums: recovery keeps
// the longest valid prefix of the log.
func (f deviceFile) flushOrdered() error {
	return f.Issue()
}

// flushBarrier makes earlier writes durable; a device has no cheaper
// ordering.
func (f deviceFile) flushBarrier() (bool, error) {
	return true, f.flushDurable()
}

// punch does nothing: a device cannot deallocate within a file, so space
// returns only when the file's tail is truncated.
func (deviceFile) punch(_, _ int64) error {
	return nil
}

// lock takes the uncontended lock at off.
func (deviceFile) lock(_ int64, _ bool) (bool, error) {
	return true, nil
}

// unlock releases the lock at off.
func (deviceFile) unlock(_ int64) error {
	return nil
}

// held reports false: no other process holds a lock.
func (deviceFile) held(_ int64) (bool, error) {
	return false, nil
}

// watch returns a watcher that waits only for stop.
func (deviceFile) watch(_ int) (watcher, error) {
	return &stopWatcher{done: make(chan struct{})}, nil
}

// close issues the collected writes.
func (f deviceFile) close() error {
	return f.Close()
}

// stopWatcher is the watcher of a file no other process changes.
type stopWatcher struct {
	// done is closed by stop.
	done chan struct{}
	// once closes done once.
	once sync.Once
}

// wait blocks until stop, reporting false.
func (w *stopWatcher) wait() bool {
	<-w.done
	return false
}

// notify does nothing; no other process reads the file.
func (*stopWatcher) notify() {}

// stop wakes wait.
func (w *stopWatcher) stop() {
	w.once.Do(func() { close(w.done) })
}

// close does nothing.
func (*stopWatcher) close() {}
