//go:build !js && !windows

package bldr_project_starlark

import "os/exec"

// startBounded starts cmd. The child limits its own memory, so there is nothing
// to release.
func startBounded(cmd *exec.Cmd, _ uint64) (func(), error) {
	return func() {}, cmd.Start()
}
