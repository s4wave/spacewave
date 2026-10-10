//go:build windows

package bldr_project_starlark

import (
	"os/exec"
	"syscall"

	winjob "github.com/aperturerobotics/go-winjob"
	"golang.org/x/sys/windows"
)

// startBounded starts cmd in a Job Object that fails any allocation beyond
// memoryBytes of committed memory in the process. The returned function closes
// the job, which also ends the process.
func startBounded(cmd *exec.Cmd, memoryBytes uint64) (func(), error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
	job, err := winjob.Start(cmd, winjob.WithKillOnJobClose(), winjob.WithProcessMemoryLimit(uintptr(memoryBytes)))
	if err != nil {
		return nil, err
	}
	return func() { _ = job.Close() }, nil
}
