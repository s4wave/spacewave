//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type bundleCacheLock struct {
	file *os.File
}

func acquireBundleCacheLock(lockPath string) (*bundleCacheLock, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "open browser bundle cache lock")
	}
	lock := &bundleCacheLock{file: file}
	if err := lock.lock(); err != nil {
		_ = file.Close()
		return nil, errors.Wrap(err, "lock browser bundle cache")
	}
	return lock, nil
}

func (lock *bundleCacheLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := lock.unlock()
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return errors.Wrap(unlockErr, "unlock browser bundle cache")
	}
	if closeErr != nil {
		return errors.Wrap(closeErr, "close browser bundle cache lock")
	}
	return nil
}
