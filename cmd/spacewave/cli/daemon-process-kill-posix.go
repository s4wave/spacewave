//go:build !js && (darwin || linux)

package spacewave_cli

import (
	"errors"
	"os/exec"
	"syscall"
)

func startDaemonCmd(cmd *exec.Cmd) error {
	return cmd.Start()
}

func releaseDaemonTree(int) error {
	return nil
}

// interruptDaemon sends SIGINT to the daemon. Its signal handler cancels the
// root context, stops its descendants, and runs cleanup.
func interruptDaemon(pid int) error {
	return syscall.Kill(pid, syscall.SIGINT)
}

// killDaemonTree forces the daemon and its descendants to exit after the
// graceful shutdown period expires.
func killDaemonTree(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
