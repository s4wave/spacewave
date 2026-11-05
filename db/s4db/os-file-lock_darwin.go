package s4db

import "github.com/s4wave/spacewave/db/s4db/filelock"

// lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
func (f *osFile) lock(off int64, wait bool) (bool, error) {
	// Intent waits must not add a process-wide Darwin deadlock edge while
	// another thread in this process can hold the writer lock.
	if off >= lockForeground && off < lockForeground+slots {
		return filelock.LockDescription(f.File, off, wait)
	}
	return filelock.Lock(f.File, off, wait)
}

// unlock releases the lock on the byte at off.
func (f *osFile) unlock(off int64) error {
	// Match the ownership used to acquire the intent byte.
	if off >= lockForeground && off < lockForeground+slots {
		return filelock.UnlockDescription(f.File, off)
	}
	return filelock.Unlock(f.File, off)
}

// held reports whether another holder has the lock on the byte at off.
func (f *osFile) held(off int64) (bool, error) {
	// Query intent using its descriptor identity rather than the process.
	if off >= lockForeground && off < lockForeground+slots {
		return filelock.HeldDescription(f.File, off)
	}
	return filelock.Held(f.File, off)
}
