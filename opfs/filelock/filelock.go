//go:build js

// Package filelock provides per-file WebLock coordination for OPFS sync
// access handles.
//
// The protocol fires a WebLock request and a createSyncAccessHandle call
// concurrently. If the handle opens before the lock is acquired, the
// handle is closed to avoid deadlocking the lock holder. Once the lock
// is held, the handle is (re)opened and returned to the caller.
package filelock

import (
	"sync"
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/pkg/errors"
)

// AcquireFile acquires an exclusive per-file WebLock and opens a sync
// access handle for the named file.
//
// The WebLock name is lockPrefix + "/" + name. If create is true the file
// is created when it does not exist.
//
// The returned release function closes the sync handle and releases the
// WebLock. It is safe to call more than once.
func AcquireFile(dir js.Value, name, lockPrefix string, create bool) (*opfs.SyncFile, func(), error) {
	lockName := lockPrefix + "/" + name

	// Start the WebLock request in the background.
	type lockResult struct {
		release func()
		err     error
	}
	lockCh := make(chan lockResult, 1)
	go func() {
		rel, err := acquireWebLock(lockName, true)
		lockCh <- lockResult{rel, err}
	}()

	// Optimistically try to open the sync handle while the lock request
	// is in flight. createSyncAccessHandle resolves quickly: it either
	// returns the handle or throws NoModificationAllowedError.
	handle, _ := openHandle(dir, name, create)

	// Check whether the lock arrived while we were opening the handle.
	var lr lockResult
	select {
	case lr = <-lockCh:
		// Lock acquired concurrently with the handle open.
	default:
		// Lock still pending. Close any handle we obtained to avoid
		// deadlocking the current lock holder who may need it.
		if handle != nil {
			handle.Close()
			handle = nil
		}
		lr = <-lockCh
	}

	if lr.err != nil {
		if handle != nil {
			handle.Close()
		}
		return nil, nil, errors.Wrap(lr.err, "acquire WebLock")
	}

	// Lock held. Open the handle if the optimistic attempt failed.
	if handle == nil {
		var err error
		handle, err = openHandle(dir, name, create)
		if err != nil {
			lr.release()
			return nil, nil, err
		}
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			handle.Close()
			lr.release()
		})
	}
	return handle, release, nil
}

// openHandle opens or creates a sync access handle.
func openHandle(dir js.Value, name string, create bool) (*opfs.SyncFile, error) {
	if create {
		return opfs.CreateSyncFile(dir, name)
	}
	return opfs.OpenSyncFile(dir, name)
}
