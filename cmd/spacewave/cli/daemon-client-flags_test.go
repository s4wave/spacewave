//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestSocketAwareCommandFamiliesExposeSocketPathFlag(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cli.Command
	}{
		{"fs ls", buildFsLsCommand()},
		{"git show", buildGitShowCommand()},
		{"git clone", buildGitCloneCommand()},
		{"canvas show", buildCanvasShowCommand()},
		{"web", newWebCommand(nil)},
		{"web list", newWebListCommand()},
		{"web stop", newWebStopCommand()},
		{"debug trace", newDebugTraceCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCommandFlags(t, tt.cmd, "state-path", "socket-path")
		})
	}
}

func assertCommandFlags(t *testing.T, cmd *cli.Command, names ...string) {
	t.Helper()

	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	for _, name := range names {
		if set.Lookup(name) == nil {
			t.Fatalf("%s flag missing", name)
		}
	}
}
