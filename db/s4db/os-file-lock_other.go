//go:build linux || windows

package s4db

import "github.com/s4wave/spacewave/db/s4db/filelock"

// lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
func (f *osFile) lock(off int64, wait bool) (bool, error) {
	return filelock.Lock(f.File, off, wait)
}

// unlock releases the lock on the byte at off.
func (f *osFile) unlock(off int64) error {
	return filelock.Unlock(f.File, off)
}

// held reports whether another holder has the lock on the byte at off.
func (f *osFile) held(off int64) (bool, error) {
	return filelock.Held(f.File, off)
}
