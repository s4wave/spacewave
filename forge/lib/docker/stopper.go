package forge_lib_docker

import (
	"context"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// Stopper stops Docker runtimes by their persisted CLI identity.
type Stopper struct {
	// runner executes the CLI with the saved path and environment.
	runner DockerRunner
}

// NewStopper constructs an idempotent Docker runtime stopper.
func NewStopper(runner DockerRunner) *Stopper {
	return &Stopper{runner: runner}
}

// StopRuntime stops the named Docker container within its saved timeout and
// removes it, confirming once the container is gone. Every Docker runtime
// ends here: its reservation's release after the Execution has read the exit
// status and logs, a cancel or drain, and the next Worker's reconcile after a
// crash left it running. Nothing reads the container after that, so removing
// it keeps finished runs from piling up on the host. A container that stays
// after a failed removal leaves the stop pending, to be retried.
func (s *Stopper) StopRuntime(ctx context.Context, rt *forge_runtime.BackendRuntimeIdentity) (bool, error) {
	// Require a Docker runtime before invoking its saved stop command.
	if rt.Backend != "docker" {
		return false, errors.Errorf("unsupported runtime backend %q", rt.Backend)
	}

	// Stop the container using its persisted CLI environment and timeout,
	// then remove it, which also ends one the stop could not.
	command := rt.StopCommand
	if command == "" {
		command = "docker"
	}
	args := []string{"stop"}
	if rt.StopTimeoutSeconds != 0 {
		args = append(args, "--time", strconv.FormatUint(uint64(rt.StopTimeoutSeconds), 10))
	}
	args = append(args, rt.Id)
	_, stopErr := s.runner.Run(ctx, command, args, rt.StopEnv)
	_, rmErr := s.runner.Run(ctx, command, []string{"rm", "--force", rt.Id}, rt.StopEnv)
	if rmErr == nil {
		return true, nil
	}

	// A failed removal may name a container already gone; list it to see.
	out, listErr := s.runner.Run(ctx, command, []string{"ps", "-a", "--filter", "name=^/" + rt.Id + "$", "--format", "{{.ID}}"}, rt.StopEnv)
	if listErr == nil && strings.TrimSpace(string(out)) == "" {
		return true, nil
	}
	if stopErr != nil {
		return false, stopErr
	}
	return false, rmErr
}

// _ is a type assertion.
var _ forge_runtime.RuntimeStopper = (*Stopper)(nil)
