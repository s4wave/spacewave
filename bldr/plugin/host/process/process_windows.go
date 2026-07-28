//go:build windows

package plugin_host_process

import (
	"os/exec"
	"syscall"

	winjob "github.com/aperturerobotics/go-winjob"
	"golang.org/x/sys/windows"
)

// preStartCmd prevents native plugin processes from allocating console windows.
func preStartCmd(cmd *exec.Cmd) (struct{}, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
	return struct{}{}, nil
}

// startCmd starts the command using a Windows Job group.
func startCmd(cmd *exec.Cmd, val struct{}) (*winjob.JobObject, error) {
	// NOTE: possible to set other kinds of limits here, too!
	// even possible to prevent writing to the clipboard.
	return winjob.Start(cmd, winjob.WithKillOnJobClose())
}

// shutdownCmd attempts to gracefully shutdown the process.
func shutdownCmd(cmd *exec.Cmd, preStartObj struct{}, startObj *winjob.JobObject) error {
	// NOTE: windows does not have SIGINT.
	// trust the plugin to begin graceful shutdown when the pipe listener closes.
	return nil
}

// killCmd attempts to kill the process immediately.
func killCmd(cmd *exec.Cmd, preStartObj struct{}, startObj *winjob.JobObject) error {
	_ = startObj.Terminate()
	_ = startObj.Close()
	return nil
}
