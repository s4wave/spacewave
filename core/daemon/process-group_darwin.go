package daemon

import (
	stderrors "errors"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const (
	// processZombie is Darwin's SZOMB process state from sys/proc.h.
	processZombie = 5
	// processExiting is Darwin's P_WEXIT flag from sys/proc.h. sysctl exports
	// it once kernel exit begins, before the process becomes a zombie.
	processExiting = 0x00002000
)

// killProcessGroup terminates the group while its unreaped leader reserves pid.
func killProcessGroup(pid int) error {
	// Darwin's killpg1 skips zombies and processes draining their references
	// during exit. It returns EPERM when no member accepts the signal.
	err := unix.Kill(-pid, unix.SIGKILL)
	if err == unix.ESRCH {
		return nil
	}
	if err != unix.EPERM {
		return err
	}

	// Distinguish an exiting group from a real permission failure.
	// This is one snapshot after the failed signal, never exit polling. The
	// retained leader prevents this group identity from being recycled.
	group, inspectErr := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pid)
	if inspectErr != nil {
		return stderrors.Join(err, inspectErr)
	}
	for _, member := range group {
		if member.Proc.P_stat != processZombie && member.Proc.P_flag&processExiting == 0 {
			return errors.Wrapf(err, "terminate group %d: member %d has state %d and flags %#x", pid, member.Proc.P_pid, member.Proc.P_stat, member.Proc.P_flag)
		}
	}
	return nil
}
