//go:build !js && (darwin || linux)

package daemon

import "os/exec"

// startProcess retains the child's detached process group for startup cleanup.
func startProcess(cmd *exec.Cmd) (*process, error) {
	// Setpgid in prepareDaemonStart makes this unreaped PID the group identity.
	// process.stop signals the group before Wait can make the PID reusable.
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Keep both signals ahead of Wait or Release so pid cannot be recycled.
	pid := cmd.Process.Pid
	return &process{
		cmd:       cmd,
		kill:      func() error { return killProcessGroup(pid) },
		killChild: cmd.Process.Kill,
		detach:    func() error { return nil },
	}, nil
}
