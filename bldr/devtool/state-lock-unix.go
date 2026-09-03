//go:build !js && unix

package devtool

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"
)

const stateLockHelperEnv = "SPACEWAVE_DEVTOOL_STATE_LOCK_HELPER"

// The helper process owns the only blocking wait; canceling its process cannot leave a waiter that later acquires the inherited lock.
func init() {
	helperParentPID, err := strconv.Atoi(os.Getenv(stateLockHelperEnv))
	if err != nil || helperParentPID != os.Getppid() {
		return
	}
	if err := unix.Flock(3, unix.LOCK_EX); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func (l *stateLock) tryLock() (bool, error) {
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if stderrors.Is(err, unix.EWOULDBLOCK) || stderrors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func (l *stateLock) lock(ctx context.Context) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable) //nolint:gosec
	cmd.Env = []string{stateLockHelperEnv + "=" + strconv.Itoa(os.Getpid())}
	cmd.ExtraFiles = []*os.File{l.file}
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = l.unlock()
		return err
	}
	return nil
}

func (l *stateLock) unlock() error {
	return unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
}
