package forge_lib_docker

import "context"

// DockerRunner executes one docker CLI command and returns its stdout.
// The env parameter is the complete subprocess environment; implementations
// must not inherit the host environment.
type DockerRunner interface {
	// Run executes the named binary with args and env, returning stdout.
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
	// Logs reads both output streams from a completed container.
	Logs(ctx context.Context, name, containerID string, env []string) ([]byte, []byte, error)
}
