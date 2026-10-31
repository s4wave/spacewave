//go:build darwin || linux || windows

package s4db

import (
	"context"
	"os"

	"github.com/aperturerobotics/util/broadcast"
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
	// bcast guards ids and wakes waiters when a claim is released.
	bcast broadcast.Broadcast
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
	var held bool
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if _, ok := r.ids[id]; ok {
			held = true
			return
		}
		r.ids[id] = struct{}{}
	})
	if held {
		return fileID{}, &openInProcessError{id: id}
	}
	return id, nil
}

// release unregisters a file claim and wakes openers waiting for it.
func (r *fileRegistry) release(id fileID) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(r.ids, id)
		broadcast()
	})
}

// waitRelease blocks until id is no longer claimed, or ctx ends.
func (r *fileRegistry) waitRelease(ctx context.Context, id fileID) error {
	return r.bcast.Wait(ctx, func(_ func(), getWaitCh func() <-chan struct{}) (bool, error) {
		if _, held := r.ids[id]; !held {
			return true, nil
		}
		_ = getWaitCh()
		return false, nil
	})
}
