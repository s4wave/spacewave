package npm

import (
	oexec "os/exec"

	"github.com/aperturerobotics/util/exec"
)

// NpmExec runs the "npm exec" command to run a npm package w/o installing.
//
// pkg is be the package name, optionally with the version:
//   - @electron/asar
//   - @electron/asar@3.2.3
func NpmExec(pkg string, cmd ...string) *oexec.Cmd {
	args := append([]string{"--quiet", "exec", "--", pkg}, cmd...)
	return exec.NewCmd("npm", args...)
}
