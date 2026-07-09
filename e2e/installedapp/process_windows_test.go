//go:build !skip_e2e && !js && windows

package installedapp

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func processGroupAttr() *syscall.SysProcAttr {
	return nil
}

func stopProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if cmd.ProcessState != nil {
		return nil
	}
	_ = cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return err
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return nil
	}
}

func processGroupAlive(pid int) bool {
	return false
}

func stopStateRootProcesses(stateRoot string) {
}
