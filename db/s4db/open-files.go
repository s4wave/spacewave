//go:build darwin || linux || windows

package s4db

import (
	"context"
	"errors"
	"os"
	"sync"

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
	ids map[fileID]os.FileInfo
	// duplicates retains descriptors opened across an external path replacement
	// until the existing owner's close, preserving its process-scoped locks.
	duplicates map[fileID][]*os.File
}

// openFiles is the registry of this process.
var openFiles = &fileRegistry{ids: make(map[fileID]os.FileInfo), duplicates: make(map[fileID][]*os.File)}

// OpenFile opens a database descriptor once per process, shared by the engine
// and format converters. opener chooses creation and platform sharing flags.
// The returned idempotent close function releases both the descriptor and its claim; the
// caller must use it instead of File.Close. Aliases fail with ErrOpenInProcess
// before opening a descriptor, so refusing them preserves the owner's locks.
func OpenFile(path string, opener func(string) (*os.File, error)) (*os.File, func() error, error) {
	// Serialize descriptor opens with registration and close. This lock covers
	// only file metadata operations, never a database read, stream or commit.
	r := openFiles
	l := r.bcast.Lock()
	defer l.Unlock()
	info, err := os.Stat(path) // #nosec G703 -- the caller chooses the database path.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	for id, held := range r.ids {
		if info != nil && os.SameFile(info, held) {
			return nil, nil, &openInProcessError{id: id}
		}
	}

	// Open and identify the actual inode, which another process may have
	// renamed over the path after Stat. Keep a duplicate until its owner closes:
	// closing it here would drop that owner's locks on Darwin.
	f, err := opener(path)
	if err != nil {
		return nil, nil, err
	}

	// Match the descriptor's identity after opening, including a concurrent rename.
	id, err := identify(f)
	if err != nil {
		return nil, nil, errors.Join(err, f.Close())
	}
	if _, held := r.ids[id]; held {
		r.duplicates[id] = append(r.duplicates[id], f)
		return nil, nil, &openInProcessError{id: id}
	}

	// Retain metadata for alias checks and return the single owning close.
	info, err = f.Stat()
	if err != nil {
		return nil, nil, errors.Join(err, f.Close())
	}
	r.ids[id] = info
	return f, sync.OnceValue(func() error { return r.close(f, id) }), nil
}

// close releases a descriptor and any duplicate seen during a path replacement,
// then wakes openers after no descriptor can still drop the old owner's locks.
func (r *fileRegistry) close(f *os.File, id fileID) error {
	// Close every descriptor before permitting another owner of this inode.
	l := r.bcast.Lock()
	defer l.Unlock()
	err := f.Close()
	for _, duplicate := range r.duplicates[id] {
		err = errors.Join(err, duplicate.Close())
	}
	delete(r.duplicates, id)
	delete(r.ids, id)
	l.Broadcast()
	return err
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
