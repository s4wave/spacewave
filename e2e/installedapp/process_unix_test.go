//go:build !skip_e2e && !js && !windows

package installedapp

import (
	"errors"
	"os/exec"
	"regexp"
	"syscall"
	"time"
)

func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func stopProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if cmd.ProcessState != nil {
		return nil
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return nil
			}
		}
		return err
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return nil
	}
}

func processGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(-pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func stopStateRootProcesses(stateRoot string) {
	pattern := regexp.QuoteMeta(stateRoot)
	_ = exec.Command("pkill", "-TERM", "-f", pattern).Run()
	// The state-dir cleanup path has no child process handle to wait on, so
	// cleanup gives pkill one bounded settle before the final SIGKILL.
	time.Sleep(500 * time.Millisecond)
	_ = exec.Command("pkill", "-KILL", "-f", pattern).Run()
}
