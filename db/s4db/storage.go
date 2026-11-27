package s4db

import "io"

// storage is the file holding a database. Reads and writes address bytes.
// The durable flush, byte locks, and the watcher coordinate durability and
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
	// evict drops the file's clean cached pages, so the next reads come from
	// the drive. A writable open flushes and then calls it before recovery
	// reads the file: a failed flush of an earlier handle may have left pages
	// marked clean that never reached the drive, and the next flush would not
	// write them. A platform or device with no such cache does nothing.
	evict() error
	// discard drops the writes the storage holds and has not sent to the
	// file, as a crash would. A failed handle calls it before close, so close
	// sends nothing a failed flush left behind. Storage that sends each write
	// at once holds none.
	discard()
	// writeback writes chunks' dirty pages to the drive and waits for them.
	// A background job calls it every bgDirtyMax from its idle thread with
	// the chunks it wrote, so its bytes leave at the idle priority and a
	// commit's durable flush never finds more than bgDirtyMax of them ahead
	// of it. A platform with no call for a range writes the whole file.
	writeback(chunks []uint64) error
	// lock takes a write lock on the byte at off, waiting for other holders
	// when wait is set. Without wait it reports false when another holder
	// has it.
	lock(off int64, wait bool) (bool, error)
	// unlock releases the lock on the byte at off.
	unlock(off int64) error
	// held reports whether another holder has the lock on the byte at off.
	held(off int64) (bool, error)
	// watch returns file-change notifications for a reader, or the writer notifier.
	watch(slot int, readOnly bool) (watcher, error)
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
