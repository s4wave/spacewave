package s4db

import "io"

// storage is the file holding a database. Reads and writes address bytes;
// the durable flush, byte locks, and the watcher coordinate durability and
// the processes sharing the file. The only ordering primitive is the durable
// flush: recovery keeps the longest valid prefix of the log and verifies the
// values of records written after the last mark, so no barrier is needed.
type storage interface {
	io.ReaderAt
	io.WriterAt

	// size returns the file length.
	size() (int64, error)
	// truncate sets the file length.
	truncate(n int64) error
	// flushDurable makes earlier writes durable on the drive.
	flushDurable() error
	// lock takes a write lock on the byte at off, waiting for other holders
	// when wait is set. Without wait it reports false when another holder
	// has it.
	lock(off int64, wait bool) (bool, error)
	// unlock releases the lock on the byte at off.
	unlock(off int64) error
	// held reports whether another holder has the lock on the byte at off.
	held(off int64) (bool, error)
	// watch returns a watcher for the reader in slot.
	watch(slot int) (watcher, error)
	// close releases the file. The database closes its watcher first.
	close() error
}

// watcher wakes a reader when other processes change the file.
type watcher interface {
	// wait blocks until the file changes, reporting false once stopped.
	wait() bool
	// notify tells other processes the file changed. The caller holds the
	// writer lock.
	notify()
	// stop wakes wait for the last time.
	stop()
	// close releases the watcher.
	close()
}
