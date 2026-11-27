package common

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
)

// sharedFiles is the database files Stores in this process hold.
//
// Closing any descriptor of a file drops every POSIX fcntl lock the process
// holds on it, including the SHARED lock SQLite holds on the database file in
// WAL mode. mattn's SQLite takes such locks, and so does modernc's unless the
// process opts into OFD locks. ncruces takes OFD locks, which belong to the
// descriptor that took them. So a Store holds its file before its pool opens,
// each connection of the pool holds it from before it opens until after it
// closes, and the descriptor closes with the last hold: no close here drops a
// lock a connection still open holds, even one a caller keeps in use past the
// Store's Close. The locks are the process's, so the table is too.
var sharedFiles = newFileTable()

// fileTable is the database files held in one process by path, each with the
// one descriptor their checkpointers write it back through. Paths are joined
// through symlinks. A hard link to a file defeats this descriptor sharing, as
// does code outside this package closing a descriptor of the file.
//
// Every hold, a Store's or a connection's, is taken through acquire, which
// finds the file's entry by path, so a file has an entry in open exactly
// while it has holds: acquire adds the entry with its first hold and release
// removes it with its last. A hold taken after a file's last release, such as
// a connection a closed pool was still opening, makes a new entry rather than
// reviving the old one, and a release can only remove its own file's entry.
//
// marks is the fail-stop mark of each database file a Store holds, matched
// by the file's identity with os.SameFile rather than by its path, so a hard
// link to a file shares its mark and a new file at the path does not. A mark
// holds the file's one kvtx.FailStop, which every Store's flusher on the file
// shares, so the file's WAL flushes run one at a time and a failure of any of
// them refuses every Store's transactions, commits, checkpoints and flushes,
// waiting out those already admitted: a Store whose own flush did not fail
// would otherwise read the frames the other's flush lost as committed, and
// its own flush of the file could succeed without them. A failed mark stays
// until the process exits: while it runs, the kernel's page cache may hold
// WAL frames the drive never received, and the failed Store's pool, left
// open, keeps the WAL index that maps them. The open pool also keeps the
// file's inode from being reused by a new file.
type fileTable struct {
	mtx   sync.Mutex
	open  map[string]*sharedFile
	marks []*fileMark
}

// newFileTable returns an empty fileTable.
func newFileTable() *fileTable {
	return &fileTable{open: make(map[string]*sharedFile)}
}

// fileMark is the fail-stop mark of one database file, shared by the Stores
// in the process that hold it.
type fileMark struct {
	// info identifies the file for os.SameFile.
	info os.FileInfo
	// refs counts the holds marked with it, guarded by the table's mtx.
	refs int
	// stop is the file's fail-stop state.
	stop *kvtx.FailStop
}

// sharedFile is one database file Stores in this process hold.
type sharedFile struct {
	// path is the file's key in its fileTable.
	path string
	// holds counts the unreleased holds, guarded by the table's mtx.
	holds int
	// fd is the descriptor checkpointers write back through, opened on first
	// use and closed with the last hold, guarded by the table's mtx.
	fd *os.File
	// retired is the descriptors fd held before create replaced it with one
	// of a new file at the path, kept open until the last hold, since their
	// close would drop the locks the process holds on the files they name.
	// Guarded by the table's mtx.
	retired []*os.File
}

// fileHold is one Store's hold on a sharedFile.
type fileHold struct {
	table *fileTable
	file  *sharedFile
	// mark is the file's fail-stop mark, set by markFile before the Store
	// that owns the hold exists and nil for a hold without one.
	mark *fileMark
	// released is set by the first release, guarded by the table's mtx.
	released bool
}

// hold holds the database file at path, nil for a path filepath.Abs rejects.
func (t *fileTable) hold(path string) *fileHold {
	// Key the file by its path with symlinks resolved, or its directory's
	// for a file not yet created.
	abs, err := filepath.Abs(path)
	if path == "" || err != nil {
		return nil
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	} else if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(dir, filepath.Base(abs))
	}

	// Count the hold on the file's entry.
	return t.acquire(abs)
}

// acquire takes a hold on the file keyed path, adding its entry with the
// first hold.
func (t *fileTable) acquire(path string) *fileHold {
	// Find or add the entry and count the hold.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	f := t.open[path]
	if f == nil {
		f = &sharedFile{path: path}
		t.open[path] = f
	}
	f.holds++
	return &fileHold{table: t, file: f}
}

// again takes another hold on the file at h's path, nil for a nil h. Once
// h's file has lost its last hold, the new hold is on a new entry.
func (h *fileHold) again() *fileHold {
	// Hold the path again through the table.
	if h == nil {
		return nil
	}
	return h.table.acquire(h.file.path)
}

// create creates the file h holds unless it exists, keeping the new file's
// descriptor as the one writeBack uses, which closes only with the file's
// last hold: closing a descriptor opened only to create the file would drop
// the locks the process holds on it. A descriptor of an earlier file at the
// path, renamed or removed since, is retired.
func (h *fileHold) create() error {
	// Leave an existing file to SQLite.
	h.table.mtx.Lock()
	defer h.table.mtx.Unlock()
	if _, err := os.Stat(h.file.path); err == nil {
		return nil
	}

	// Create the file, unless another process just did.
	fd, err := os.OpenFile(h.file.path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if h.file.fd != nil {
		h.file.retired = append(h.file.retired, h.file.fd)
	}
	h.file.fd = fd
	return nil
}

// writeBack writes the file's dirty pages back through its descriptor,
// opening file, the name SQLite reports for it, on first use. The caller holds
// h unreleased, so the descriptor stays open while it writes.
func (h *fileHold) writeBack(file string) {
	// Take the descriptor, opening it once.
	if h == nil {
		return
	}
	h.table.mtx.Lock()
	if h.file.fd == nil {
		fd, err := os.OpenFile(file, os.O_RDWR, 0)
		if err != nil {
			h.table.mtx.Unlock()
			return
		}
		h.file.fd = fd
	}
	fd := h.file.fd
	h.table.mtx.Unlock()
	_ = writeBackFile(fd)
}

// markFile takes the fail-stop mark of the file h holds, which must exist,
// for h to keep until its release. The marks are few, one per file the
// process holds, so it scans them.
func (h *fileHold) markFile() error {
	// Identify the file the path names now. On Windows os.SameFile reads the
	// file's identity through its path on first use, so compare it with
	// itself while the path still names it.
	info, err := os.Stat(h.file.path)
	if err != nil {
		return err
	}
	os.SameFile(info, info)

	// Find or add its mark and count the hold.
	h.table.mtx.Lock()
	defer h.table.mtx.Unlock()
	var m *fileMark
	for _, mark := range h.table.marks {
		if os.SameFile(mark.info, info) {
			m = mark
			break
		}
	}
	if m == nil {
		m = &fileMark{info: info, stop: kvtx.NewFailStop()}
		h.table.marks = append(h.table.marks, m)
	}
	m.refs++
	h.mark = m
	return nil
}

// failStop returns the fail-stop state of h's file, shared by every Store on
// it, or a new one for a nil h or a hold without a mark.
func (h *fileHold) failStop() *kvtx.FailStop {
	if h == nil || h.mark == nil {
		return kvtx.NewFailStop()
	}
	return h.mark.stop
}

// release drops the hold, closing the descriptors with the file's last. Only
// the first release of h counts. It follows the close of the pool that held
// the file, or the end of the Store's use of a pool left open after a failed
// WAL flush, whose connections keep their own holds and so the descriptor.
func (h *fileHold) release() error {
	// Count the hold out once.
	if h == nil {
		return nil
	}
	h.table.mtx.Lock()
	defer h.table.mtx.Unlock()
	if h.released {
		return nil
	}
	h.released = true
	if m := h.mark; m != nil {
		if m.refs--; m.refs == 0 && m.stop.Failed() == nil {
			h.table.marks = slices.DeleteFunc(h.table.marks, func(mark *fileMark) bool { return mark == m })
		}
	}
	f := h.file
	if f.holds--; f.holds > 0 {
		return nil
	}

	// Remove the entry, its own while it has holds, and close the descriptors
	// with the last hold.
	delete(h.table.open, f.path)
	var err error
	for _, fd := range append(f.retired, f.fd) {
		if fd != nil {
			err = errors.Join(err, fd.Close())
		}
	}
	f.fd, f.retired = nil, nil
	return err
}
