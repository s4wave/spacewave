package forge_lib_docker

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// DockerRunner executes one docker CLI command and returns its stdout.
// The env parameter is the complete subprocess environment; implementations
// must not inherit the host environment.
type DockerRunner interface {
	// Run executes the named binary with args and env, returning stdout.
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
}

// ExecDockerRunner runs docker CLI commands as subprocesses with an
// explicitly constructed environment. Stdout is returned for parsing;
// stderr appears only in failure errors so docker warnings cannot corrupt
// parsed values such as container ids.
type ExecDockerRunner struct{}

// NewExecDockerRunner constructs a subprocess docker runner.
func NewExecDockerRunner() *ExecDockerRunner {
	return &ExecDockerRunner{}
}

// Run executes the named binary with args and env, returning stdout.
func (r *ExecDockerRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return stdout.Bytes(), context.Canceled
		}
		return stdout.Bytes(), errors.Errorf("%s %s failed: %s: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// _ is a type assertion
var _ DockerRunner = (*ExecDockerRunner)(nil)
