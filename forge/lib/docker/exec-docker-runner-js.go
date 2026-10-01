//go:build js

package forge_lib_docker

import (
	"context"

	"github.com/pkg/errors"
)

// ErrDockerUnavailable is returned by the browser runner, which cannot start
// subprocesses.
var ErrDockerUnavailable = errors.New("docker is unavailable in a browser runtime")

// ExecDockerRunner reports that a browser runtime has no docker CLI.
type ExecDockerRunner struct{}

// NewExecDockerRunner constructs the browser docker runner.
func NewExecDockerRunner() *ExecDockerRunner {
	return &ExecDockerRunner{}
}

// Run returns ErrDockerUnavailable.
func (r *ExecDockerRunner) Run(context.Context, string, []string, []string) ([]byte, error) {
	return nil, ErrDockerUnavailable
}

// Logs returns ErrDockerUnavailable.
func (r *ExecDockerRunner) Logs(context.Context, string, string, []string) ([]byte, []byte, error) {
	return nil, nil, ErrDockerUnavailable
}

// _ is a type assertion
var _ DockerRunner = (*ExecDockerRunner)(nil)
