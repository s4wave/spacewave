//go:build !js

package harness

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildLock owns one held cross-process build lock.
type BuildLock struct {
	file    *os.File
	pidPath string
}

// AcquireBuildLock acquires a named cross-process build lock. Context
// cancellation is checked before and after the kernel wait, but cannot promptly
// interrupt a caller parked in that wait.
func AcquireBuildLock(ctx context.Context, le *logrus.Entry, lockDir, name string) (*BuildLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return nil, errors.Errorf("invalid build lock name %q", name)
	}
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create build lock directory")
	}
	lockPath := filepath.Join(lockDir, name+".lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "open build lock")
	}
	lock := &BuildLock{file: file, pidPath: lockPath + ".pid"}
	locked, err := lock.tryLock()
	if err != nil {
		lock.closeAfterError()
		return nil, errors.Wrap(err, "lock build")
	}
	if !locked {
		message := "waiting for another process to release build lock: " + lockDir
		if pid := lock.readHolderPID(); pid != "" {
			message = "waiting for pid " + pid + " to release build lock: " + lockDir
		}
		if le != nil {
			le.Warn(message)
		} else {
			_, _ = os.Stderr.WriteString("warning: " + message + "\n")
		}
		if err := lock.lock(); err != nil {
			lock.closeAfterError()
			return nil, errors.Wrap(err, "wait for build lock")
		}
	}
	if err := ctx.Err(); err != nil {
		lock.Release()
		return nil, err
	}
	pid := strconv.AppendInt(nil, int64(os.Getpid()), 10)
	pid = append(pid, '\n')
	if err := os.WriteFile(lock.pidPath, pid, 0o644); err != nil {
		lock.Release()
		return nil, errors.Wrap(err, "write build lock pid")
	}
	return lock, nil
}

// Release unlocks and closes the build lock without unlinking its shared path.
func (l *BuildLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.unlock()
	_ = l.file.Close()
	l.file = nil
}

func (l *BuildLock) readHolderPID() string {
	data, err := os.ReadFile(l.pidPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (l *BuildLock) closeAfterError() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	l.file = nil
}
