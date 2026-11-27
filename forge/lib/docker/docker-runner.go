package forge_lib_docker

import (
	"context"
	"io"
)

// DockerRunner executes docker CLI commands.
// The env parameter is the complete subprocess environment. Implementations
// must not inherit the host environment.
type DockerRunner interface {
	// Run executes the named binary with args and env, returning stdout.
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
	// Stream executes the named binary with args and env, copying its stdout
	// and stderr to the writers as the command writes them.
	Stream(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error
}
