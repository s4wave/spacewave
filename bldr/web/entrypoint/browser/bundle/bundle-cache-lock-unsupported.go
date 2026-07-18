//go:build !js && !unix && !windows

package entrypoint_browser_bundle

import "sync"

var unsupportedBundleCacheLock sync.Mutex

func (lock *bundleCacheLock) lock() error {
	unsupportedBundleCacheLock.Lock()
	return nil
}

func (lock *bundleCacheLock) unlock() error {
	unsupportedBundleCacheLock.Unlock()
	return nil
}
