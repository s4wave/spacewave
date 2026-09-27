package daemon

import "golang.org/x/sys/unix"

// killProcessGroup terminates the group while its unreaped leader reserves pid.
func killProcessGroup(pid int) error {
	err := unix.Kill(-pid, unix.SIGKILL)
	if err == unix.ESRCH {
		return nil
	}
	return err
}
