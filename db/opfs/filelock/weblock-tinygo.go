//go:build js && tinygo

package filelock

import "github.com/s4wave/spacewave/db/opfs"

// AcquireWebLock requests a WebLock via navigator.locks.request.
// The returned function releases the lock. It is safe to call once.
func AcquireWebLock(name string, exclusive bool) (func(), error) {
	return opfs.AcquireWebLock(name, exclusive)
}

// AcquireWebLockIfAvailable requests a WebLock without waiting.
// If the lock is unavailable, acquired is false and release is nil.
func AcquireWebLockIfAvailable(name string, exclusive bool) (release func(), acquired bool, err error) {
	return opfs.AcquireWebLockIfAvailable(name, exclusive)
}
