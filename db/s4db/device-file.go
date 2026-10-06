package s4db

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/volume/device"
)

// deviceFile is a database file on a device. A device has one opener, so
// no other process shares the file: every lock is free and no watcher
// fires.
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

// flushDurable issues the collected writes with a device flush.
func (f deviceFile) flushDurable() error {
	return f.Sync()
}

// flushOrdered does nothing. A device keeps its calls in order but a crash
// may tear any unflushed write, so ordered commits rely on record
// checksums: recovery keeps the longest valid prefix of the log.
func (deviceFile) flushOrdered() error {
	return nil
}

// flushBarrier makes earlier writes durable; a device has no cheaper
// ordering.
func (f deviceFile) flushBarrier() (bool, error) {
	return true, f.Sync()
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
