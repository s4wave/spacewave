//go:build darwin || linux

package plugin_host_process

import (
	"os/exec"
	"syscall"
)

// preStartCmd sets the Setpgid flag so that the sub-process has a process group.
// we later kill the process group to kill the process & all child processes.
func preStartCmd(cmd *exec.Cmd) (struct{}, error) {
	// set pgid so that we can kill the entire process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return struct{}{}, nil
}

// startCmd starts the command.
func startCmd(cmd *exec.Cmd, preStartObj struct{}) (int, error) {
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// shutdownCmd attempts to gracefully shutdown the process.
func shutdownCmd(cmd *exec.Cmd, preStartObj struct{}, pid int) error {
	// graceful shutdown: send sigint to pgroup
	_ = syscall.Kill(-pid, syscall.SIGINT)
	return nil
}

// killCmd attempts to kill the process immediately.
func killCmd(cmd *exec.Cmd, preStartObj struct{}, pid int) error {
	// kill pgid as well for child processes
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	// kill the process using Go api as well
	_ = cmd.Process.Kill()
	return nil
}
