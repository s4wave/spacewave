package s4db

import (
	"encoding/binary"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// processInotify is the process's one inotify instance. The kernel limits
// instances per user across the whole host, by default to 128, while watches
// are plentiful, so every database file adds a watch here rather than taking
// an instance of its own.
var processInotify inotifyInstance

// inotifyInstance routes the change events of one inotify instance to the
// watcher of each file. It starts with its first watch and lives as long as
// the process.
type inotifyInstance struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// fd is the inotify descriptor.
	fd int
	// watchers maps each watch descriptor to the watcher of its file, or is
	// nil before the first watch. A process opens a database file once, so a
	// descriptor has one watcher.
	watchers map[int]*fileWatcher
}

// add watches the file at path for writes, waking w on each.
func (i *inotifyInstance) add(path string, w *fileWatcher) error {
	// Serialize with the reader and other watches.
	i.mtx.Lock()
	defer i.mtx.Unlock()

	// Open the instance and start its reader with the first watch.
	if i.watchers == nil {
		fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
		if err != nil {
			return err
		}
		i.fd = fd
		i.watchers = make(map[int]*fileWatcher)
		go i.read(os.NewFile(uintptr(fd), "inotify")) // #nosec G115 -- descriptors are not negative.
	}

	// Watch the file.
	wd, err := unix.InotifyAddWatch(i.fd, path, unix.IN_MODIFY)
	if err != nil {
		return err
	}
	w.wd = wd
	i.watchers[wd] = w
	return nil
}

// remove stops watching the file of w. The kernel has already dropped the
// watch of a deleted file, so its error is ignored.
func (i *inotifyInstance) remove(w *fileWatcher) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	delete(i.watchers, w.wd)
	_, _ = unix.InotifyRmWatch(i.fd, uint32(w.wd)) // #nosec G115 -- watch descriptors are positive.
}

// read wakes the watcher of each changed file. The non-blocking descriptor
// waits in the runtime poller rather than a thread. A read fails only for a
// buffer smaller than one event, which the buffer size excludes.
func (i *inotifyInstance) read(file *os.File) {
	buf := make([]byte, 64<<10)
	for {
		n, err := file.Read(buf)
		if err != nil {
			return
		}

		// Wake the watcher of each event's file. A queue overflow lost events
		// for any file, so it wakes every watcher.
		i.mtx.Lock()
		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			wd := int(int32(binary.NativeEndian.Uint32(buf[off:]))) // #nosec G115 -- the kernel writes a signed descriptor.
			mask := binary.NativeEndian.Uint32(buf[off+4:])
			off += unix.SizeofInotifyEvent + int(binary.NativeEndian.Uint32(buf[off+12:]))
			if mask&unix.IN_Q_OVERFLOW != 0 {
				for _, w := range i.watchers {
					w.wake()
				}
				continue
			}
			if w := i.watchers[wd]; w != nil {
				w.wake()
			}
		}
		i.mtx.Unlock()
	}
}
