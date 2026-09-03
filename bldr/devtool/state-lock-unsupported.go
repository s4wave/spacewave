//go:build !js && !unix && !windows

package devtool

import (
	"context"

	"github.com/pkg/errors"
)

func (l *stateLock) tryLock() (bool, error) {
	return false, errors.New("bldr state locking is unsupported on this platform")
}

func (l *stateLock) lock(context.Context) error {
	return errors.New("bldr state locking is unsupported on this platform")
}

func (l *stateLock) unlock() error {
	return nil
}
