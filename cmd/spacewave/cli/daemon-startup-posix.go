//go:build !js && (darwin || linux)

package spacewave_cli

import (
	"os/exec"
	"syscall"
)

// prepareDaemonStart detaches the daemon child into its own process group.
func prepareDaemonStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return nil
}
