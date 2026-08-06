//go:build (!unix && !windows) || plan9

package npm

import "errors"

var errInstallLockUnsupported = errors.New("install lock: unsupported platform")

func (l *installLock) tryLock() (bool, error) {
	return false, errInstallLockUnsupported
}

func (l *installLock) Unlock() error {
	return errInstallLockUnsupported
}
