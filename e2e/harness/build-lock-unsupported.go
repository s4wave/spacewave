//go:build !js && !unix && !windows

package harness

import "github.com/pkg/errors"

func (l *BuildLock) tryLock() (bool, error) {
	return false, errors.New("build locking is unsupported on this platform")
}

func (l *BuildLock) lock() error {
	return errors.New("build locking is unsupported on this platform")
}

func (l *BuildLock) unlock() error {
	return nil
}
