package npm

import (
	oexec "os/exec"

	"github.com/aperturerobotics/util/exec"
)

// NpmFlags are common npm flags passed to npm commands.
var NpmFlags = []string{
	"--loglevel=error",
	"--no-progress",
}

// NpmExec runs the "npm exec" command to run a npm package w/o installing.
//
// pkg is be the package name, optionally with the version:
//   - @electron/asar
//   - @electron/asar@3.2.3
func NpmExec(pkg string, cmd ...string) *oexec.Cmd {
	args := []string{"exec"}
	args = append(args, NpmFlags...)
	args = append(args, "--", pkg)
	args = append(args, cmd...)
	return exec.NewCmd("npm", args...)
}
