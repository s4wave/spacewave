//go:build js

// Package filelock provides per-file WebLock coordination for OPFS file
// access. Acquires an exclusive WebLock per file, then opens either a sync
// access handle (DedicatedWorker) or an async file handle (SharedWorker,
// main thread) depending on context.
package filelock

import (
	"sync"
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/pkg/errors"
)

// File is the interface returned by AcquireFile. Callers use this for
// both sync and async OPFS access transparently.
type File interface {
	ReadAt(p []byte, off int64) (int, error)
	WriteAt(p []byte, off int64) (int, error)
	Size() (int64, error)
	Truncate(size int64) error
	Flush() error
}

// AcquireFile acquires an exclusive per-file WebLock and opens the named
// file for read/write access.
//
// The WebLock name is lockPrefix + "/" + name. If create is true the file
// is created when it does not exist.
//
// In DedicatedWorker contexts, uses a sync access handle (~40% faster).
// In SharedWorker/main thread contexts, uses async file access.
//
// The returned release function closes the file and releases the WebLock.
// It is safe to call more than once.
func AcquireFile(dir js.Value, name, lockPrefix string, create bool) (File, func(), error) {
	lockName := lockPrefix + "/" + name

	// Acquire exclusive WebLock.
	lockRelease, err := AcquireWebLock(lockName, true)
	if err != nil {
		return nil, nil, errors.Wrap(err, "acquire WebLock")
	}

	if opfs.SyncAvailable() {
		handle, err := openHandle(dir, name, create)
		if err != nil {
			lockRelease()
			return nil, nil, err
		}
		var once sync.Once
		release := func() {
			once.Do(func() {
				handle.Close()
				lockRelease()
			})
		}
		return &syncAdapter{handle}, release, nil
	}

	// Async path for SharedWorker/main thread.
	var af *opfs.AsyncFile
	if create {
		af, err = opfs.CreateAsyncFile(dir, name)
	} else {
		af, err = opfs.OpenAsyncFile(dir, name)
	}
	if err != nil {
		lockRelease()
		return nil, nil, err
	}
	var once sync.Once
	release := func() {
		once.Do(func() { lockRelease() })
	}
	return &asyncAdapter{af}, release, nil
}

// openHandle opens or creates a sync access handle.
func openHandle(dir js.Value, name string, create bool) (*opfs.SyncFile, error) {
	if create {
		return opfs.CreateSyncFile(dir, name)
	}
	return opfs.OpenSyncFile(dir, name)
}

// syncAdapter wraps a SyncFile to implement File.
type syncAdapter struct {
	f *opfs.SyncFile
}

func (a *syncAdapter) ReadAt(p []byte, off int64) (int, error) { return a.f.ReadAt(p, off) }
func (a *syncAdapter) WriteAt(p []byte, off int64) (int, error) { return a.f.WriteAt(p, off) }
func (a *syncAdapter) Size() (int64, error)                     { return a.f.Size(), nil }
func (a *syncAdapter) Truncate(size int64) error                { a.f.Truncate(size); return nil }
func (a *syncAdapter) Flush() error                             { a.f.Flush(); return nil }

// asyncAdapter wraps an AsyncFile to implement File.
type asyncAdapter struct {
	f *opfs.AsyncFile
}

func (a *asyncAdapter) ReadAt(p []byte, off int64) (int, error) { return a.f.ReadAt(p, off) }
func (a *asyncAdapter) WriteAt(p []byte, off int64) (int, error) { return a.f.WriteAt(p, off) }
func (a *asyncAdapter) Size() (int64, error)                     { return a.f.Size() }
func (a *asyncAdapter) Truncate(size int64) error                { return a.f.Truncate(size) }
func (a *asyncAdapter) Flush() error                             { return nil }
