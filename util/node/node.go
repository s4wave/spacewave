package node

import (
	"context"
	oexec "os/exec"

	"github.com/aperturerobotics/util/exec"
)

// NodeFlags are common node flags passed to node commands.
var NodeFlags = []string{
	"--enable-source-maps",
	"--experimental-strip-types",
}

// NodeExec builds a command to run a script with Node.
func NodeExec(ctx context.Context, filePath string, fileArgs ...string) *oexec.Cmd {
	args := []string{}
	args = append(args, NodeFlags...)
	args = append(args, filePath)
	if len(fileArgs) != 0 {
		args = append(args, "--")
		args = append(args, fileArgs...)
	}
	return exec.NewCmd(ctx, "node", args...)
}
