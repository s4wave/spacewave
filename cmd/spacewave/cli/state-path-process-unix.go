//go:build !js && !wasip1 && !windows && !plan9

package spacewave_cli

import "syscall"

func statePathLeaseProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
