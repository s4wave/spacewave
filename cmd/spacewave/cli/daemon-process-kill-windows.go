//go:build !js && windows

package spacewave_cli

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"sync"

	winjob "github.com/aperturerobotics/go-winjob"
)

var daemonJobs sync.Map

func startDaemonCmd(cmd *exec.Cmd) error {
	job, err := winjob.Start(cmd)
	if err != nil {
		return err
	}
	daemonJobs.Store(cmd.Process.Pid, job)
	return nil
}

func releaseDaemonTree(pid int) error {
	value, ok := daemonJobs.LoadAndDelete(pid)
	if !ok {
		return nil
	}
	return value.(*winjob.JobObject).Close()
}

// interruptDaemon asks taskkill to stop the daemon process tree.
func interruptDaemon(pid int) error {
	ctx, cancel := context.WithTimeout(context.Background(), daemonShutdownGracePeriod)
	defer cancel()
	return exec.CommandContext(ctx, "taskkill", "/T", "/PID", strconv.Itoa(pid)).Run()
}

// killDaemonTree forces every process retained in the daemon's Job object to
// exit. taskkill remains a fallback for commands started before the Job was
// registered.
func killDaemonTree(pid int) error {
	if value, ok := daemonJobs.LoadAndDelete(pid); ok {
		job := value.(*winjob.JobObject)
		return errors.Join(job.Terminate(), job.Close())
	}
	ctx, cancel := context.WithTimeout(context.Background(), daemonForcedShutdownTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
