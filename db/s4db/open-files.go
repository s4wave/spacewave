package s4db

import (
	"os"
	"sync"
)

// fileID identifies a file independent of its path.
type fileID struct {
	// volume is the device or volume holding the file.
	volume uint64
	// index is the file's number on the volume.
	index uint64
}

// fileRegistry tracks the database files a process has open. A process
// opens each file once: record locks on darwin belong to the process, so a
// second handle neither conflicts with the first nor keeps its locks when
// the first closes.
type fileRegistry struct {
	// mtx guards ids.
	mtx sync.Mutex
	// ids holds the open files.
	ids map[fileID]struct{}
}

// openFiles is the registry of this process.
var openFiles = &fileRegistry{ids: make(map[fileID]struct{})}

// claim registers f as open, failing with ErrOpenInProcess when it already
// is.
func (r *fileRegistry) claim(f *os.File) (fileID, error) {
	// Identify the file.
	id, err := identify(f)
	if err != nil {
		return fileID{}, err
	}

	// Register it unless already open.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if _, ok := r.ids[id]; ok {
		return fileID{}, ErrOpenInProcess
	}
	r.ids[id] = struct{}{}
	return id, nil
}

// release unregisters a file claim registered.
func (r *fileRegistry) release(id fileID) {
	r.mtx.Lock()
	delete(r.ids, id)
	r.mtx.Unlock()
}
