//go:build !js

package devtool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const stateLockFileName = "state.lock"

type stateLock struct {
	file    *os.File
	pidPath string
}

func acquireStateLock(ctx context.Context, le *logrus.Entry, stateRoot string) (*stateLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(stateRoot, stateLockFileName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "open bldr state lock")
	}
	lock := &stateLock{
		file:    file,
		pidPath: lockPath + ".pid",
	}

	locked, err := lock.tryLock()
	if err != nil {
		lock.closeAfterError()
		return nil, errors.Wrap(err, "lock bldr state")
	}
	if !locked {
		logStateLockWait(le, lock.waitMessage(stateRoot))
		if err := lock.lock(); err != nil {
			lock.closeAfterError()
			return nil, errors.Wrap(err, "wait for bldr state lock")
		}
	}
	if err := ctx.Err(); err != nil {
		lock.release()
		return nil, err
	}
	if err := lock.writePID(); err != nil {
		lock.release()
		return nil, err
	}
	return lock, nil
}

func (l *stateLock) waitMessage(stateRoot string) string {
	if pid := l.readHolderPID(); pid != "" {
		return "waiting for pid " + pid + " to release bldr state lock: " + stateRoot
	}
	return "waiting for another bldr process to release state lock: " + stateRoot
}

func (l *stateLock) writePID() error {
	pid := strconv.AppendInt(nil, int64(os.Getpid()), 10)
	pid = append(pid, '\n')
	if err := os.WriteFile(l.pidPath, pid, 0o644); err != nil {
		return errors.Wrap(err, "write bldr state lock pid")
	}
	return nil
}

func (l *stateLock) readHolderPID() string {
	data, err := os.ReadFile(l.pidPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (l *stateLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.unlock()
	_ = l.file.Close()
	l.file = nil
}

func (l *stateLock) closeAfterError() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	l.file = nil
}

func logStateLockWait(le *logrus.Entry, message string) {
	if le != nil {
		le.Warn(message)
		return
	}
	_, _ = os.Stderr.WriteString("warning: " + message + "\n")
}
