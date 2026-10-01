//go:build !js

package forge_lib_docker

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// ExecDockerRunner runs docker CLI commands as subprocesses with an
// explicitly constructed environment. Run returns stdout for parsing and
// includes stderr in failure errors, so Docker warnings cannot corrupt parsed
// values such as container IDs. Logs returns both streams separately.
type ExecDockerRunner struct{}

// NewExecDockerRunner constructs a subprocess docker runner.
func NewExecDockerRunner() *ExecDockerRunner {
	return &ExecDockerRunner{}
}

// Run executes the named binary with args and env, returning stdout.
func (r *ExecDockerRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	stdout, _, err := r.run(ctx, name, args, env)
	return stdout, err
}

// Logs reads a container's stdout and stderr without mixing the streams.
func (r *ExecDockerRunner) Logs(ctx context.Context, name, containerID string, env []string) ([]byte, []byte, error) {
	return r.run(ctx, name, []string{"logs", containerID}, env)
}

// run executes a docker command with separate captured output streams.
func (r *ExecDockerRunner) run(ctx context.Context, name string, args []string, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return stdout.Bytes(), stderr.Bytes(), context.Canceled
		}
		return stdout.Bytes(), stderr.Bytes(), errors.Errorf("%s %s failed: %s: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

// _ is a type assertion
var _ DockerRunner = (*ExecDockerRunner)(nil)
