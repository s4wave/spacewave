//go:build !js && windows

package daemon

import (
	stderrors "errors"
	"os"
	"os/exec"

	winjob "github.com/aperturerobotics/go-winjob"
	"golang.org/x/sys/windows"
)

// startProcess retains this child's Job directly, without a process registry.
func startProcess(cmd *exec.Cmd) (*process, error) {
	// Close releases the Job handle without killing its processes. Kill-on-close
	// must stay disabled because readiness transfers the daemon's lifetime.
	job, err := winjob.Start(cmd)
	if err != nil {
		return nil, err
	}

	// Retain the Job and process handles through termination and joining.
	return &process{
		cmd:       cmd,
		kill:      job.Terminate,
		killChild: func() error { return killChild(cmd.Process) },
		detach:    job.Close,
	}, nil
}

// killChild makes one termination attempt against the retained process handle.
// A signaled exit event establishes os.ErrProcessDone even when Windows reports
// access denied for termination of an already-exited process.
func killChild(child *os.Process) error {
	// TerminateProcess is asynchronous; success authorizes the caller to join.
	err := child.Kill()
	if err == nil {
		return nil
	}

	// Inspect the retained exit event once after failure, without waiting or
	// reopening a numeric PID. An unsignaled event cannot justify a join.
	var status uint32
	var waitErr error
	handleErr := child.WithHandle(func(handle uintptr) {
		status, waitErr = windows.WaitForSingleObject(windows.Handle(handle), 0)
	})
	if handleErr == nil && waitErr == nil && status == windows.WAIT_OBJECT_0 {
		return os.ErrProcessDone
	}
	return stderrors.Join(err, handleErr, waitErr)
}
