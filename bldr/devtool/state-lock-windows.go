//go:build !js && windows

package devtool

import (
	"context"
	stderrors "errors"

	"golang.org/x/sys/windows"
)

func (l *stateLock) tryLock() (bool, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(l.file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if err == nil {
		return true, nil
	}
	if stderrors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func (l *stateLock) lock(ctx context.Context) error {
	lockEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(lockEvent)
	cancelEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(cancelEvent)

	overlapped := windows.Overlapped{HEvent: lockEvent}
	handle := windows.Handle(l.file.Fd())
	err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	if err == nil {
		if err := ctx.Err(); err != nil {
			_ = l.unlock()
			return err
		}
		return nil
	}
	if !stderrors.Is(err, windows.ERROR_IO_PENDING) {
		return err
	}

	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			_ = windows.SetEvent(cancelEvent)
		case <-stopWatch:
		}
	}()
	event, waitErr := windows.WaitForMultipleObjects([]windows.Handle{lockEvent, cancelEvent}, false, windows.INFINITE)
	close(stopWatch)
	<-watchDone
	if waitErr != nil {
		_ = windows.CancelIoEx(handle, &overlapped)
		var transferred uint32
		_ = windows.GetOverlappedResult(handle, &overlapped, &transferred, true)
		return waitErr
	}
	if event == windows.WAIT_OBJECT_0 {
		var transferred uint32
		if err := windows.GetOverlappedResult(handle, &overlapped, &transferred, false); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			_ = l.unlock()
			return err
		}
		return nil
	}

	_ = windows.CancelIoEx(handle, &overlapped)
	var transferred uint32
	operationErr := windows.GetOverlappedResult(handle, &overlapped, &transferred, true)
	if operationErr == nil {
		_ = l.unlock()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return context.Canceled
}

func (l *stateLock) unlock() error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(
		windows.Handle(l.file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
}
