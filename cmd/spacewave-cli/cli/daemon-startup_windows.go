//go:build !js && windows

package spacewave_cli

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// prepareDaemonStart detaches the daemon child from the launching console.
func prepareDaemonStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
	return nil
}
