//go:build windows

package launcher_helper

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func prepareHelperCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
}
