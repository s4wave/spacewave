package common

import (
	"os"
	"path/filepath"
	"sync"
)

// sharedFiles is the database files Stores in this process hold.
//
// Closing any descriptor of a file drops every POSIX fcntl lock the process
// holds on it, including the SHARED lock SQLite holds on the database file in
// WAL mode. mattn's SQLite takes such locks, and so does modernc's unless the
// process opts into OFD locks; ncruces takes OFD locks, which belong to the
// descriptor that took them. So a Store holds its file before its pool opens,
// each connection of the pool holds it from before it opens until after it
// closes, and the descriptor closes with the last hold: no close here drops a
// lock a connection still open holds, even one a caller keeps in use past the
// Store's Close. The locks are the process's, so the table is too.
var sharedFiles = newFileTable()

// fileTable is the database files held in one process by path, each with the
// one descriptor their checkpointers write it back through. Paths are joined
// through symlinks; a hard link to a file defeats this, as does code outside
// this package closing a descriptor of the file.
//
// Every hold, a Store's or a connection's, is taken through acquire, which
// finds the file's entry by path, so a file has an entry in open exactly
// while it has holds: acquire adds the entry with its first hold and release
// removes it with its last. A hold taken after a file's last release, such as
// a connection a closed pool was still opening, makes a new entry rather than
// reviving the old one, and a release can only remove its own file's entry.
type fileTable struct {
	mtx  sync.Mutex
	open map[string]*sharedFile
}

// newFileTable returns an empty fileTable.
func newFileTable() *fileTable {
	return &fileTable{open: make(map[string]*sharedFile)}
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
}

// fileHold is one Store's hold on a sharedFile.
type fileHold struct {
	table *fileTable
	file  *sharedFile
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

// release drops the hold, closing the descriptor with the file's last. Only
// the first release of h counts, and it must follow the close of the pool
// that held the file.
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
	f := h.file
	if f.holds--; f.holds > 0 {
		return nil
	}

	// Remove the entry, its own while it has holds, and close the descriptor
	// with the last hold.
	delete(h.table.open, f.path)
	fd := f.fd
	f.fd = nil
	if fd == nil {
		return nil
	}
	return fd.Close()
}
