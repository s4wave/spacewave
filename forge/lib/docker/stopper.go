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

// StopRuntime confirms that the named Docker container has stopped or gone.
func (s *Stopper) StopRuntime(ctx context.Context, rt forge_runtime.BackendRuntimeIdentity) (bool, error) {
	if rt.Backend != "docker" {
		return false, errors.Errorf("unsupported runtime backend %q", rt.Backend)
	}
	command := rt.StopCommand
	if command == "" {
		command = "docker"
	}
	args := []string{"stop"}
	if rt.StopTimeoutSeconds != 0 {
		args = append(args, "--time", strconv.FormatUint(uint64(rt.StopTimeoutSeconds), 10))
	}
	args = append(args, rt.ID)
	_, stopErr := s.runner.Run(ctx, command, args, rt.StopEnv)
	if stopErr == nil {
		return true, nil
	}

	// A failed stop may race with ordinary exit; inspect the actual container.
	out, inspectErr := s.runner.Run(ctx, command, []string{"inspect", "--format", "{{.State.Running}}", rt.ID}, rt.StopEnv)
	if inspectErr == nil && strings.TrimSpace(string(out)) == "false" {
		return true, nil
	}
	if inspectErr == nil {
		return false, stopErr
	}
	out, listErr := s.runner.Run(ctx, command, []string{"ps", "-a", "--filter", "name=^/" + rt.ID + "$", "--format", "{{.ID}}"}, rt.StopEnv)
	if listErr == nil && strings.TrimSpace(string(out)) == "" {
		return true, nil
	}
	return false, stopErr
}

// _ is a type assertion.
var _ forge_runtime.RuntimeStopper = (*Stopper)(nil)
