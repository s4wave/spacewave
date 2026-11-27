//go:build unix

package forge_lib_skiffos

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// stopGrace is how long a canceled build may take to exit after SIGTERM.
const stopGrace = 30 * time.Second

// runMake runs make with the targets in dir, appending its output to logPath.
// The build runs in its own process group, so canceling ctx stops every
// compiler it started and not only make.
func runMake(ctx context.Context, dir, logPath string, env []string, targets ...string) error {
	// Append the build output to the workspace's log.
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	// Run make in its own process group. Cancel signals the whole group.
	cmd := exec.CommandContext(ctx, "make", targets...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = stopGrace
	return cmd.Run()
}
