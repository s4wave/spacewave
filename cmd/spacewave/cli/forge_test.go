//go:build !js

package spacewave_cli

import (
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestForgeCommandsKeepIndependentNameFlags(t *testing.T) {
	cmd := newForgeCommand(nil)
	assertFlagUsage := func(commandName, wantUsage string) {
		t.Helper()
		command := findTestSubcommand(t, cmd, commandName)
		for _, flag := range command.Flags {
			if flag.Names()[0] == "name" {
				stringFlag, ok := flag.(*cli.StringFlag)
				if !ok {
					t.Fatalf("%s --name is %T, want *cli.StringFlag", commandName, flag)
				}
				if got := stringFlag.Usage; got != wantUsage {
					t.Fatalf("%s --name usage = %q, want %q", commandName, got, wantUsage)
				}
				return
			}
		}
		t.Fatalf("%s has no --name flag", commandName)
	}
	assertFlagUsage("create-cluster", "cluster name")
	assertFlagUsage("create-job", "job name")
}

func TestForgeCreateWorkerExposesSessionPeerAndClusterInputs(t *testing.T) {
	cmd := newForgeCommand(nil)
	createWorker := findTestSubcommand(t, cmd, "create-worker")
	assertCommandFlags(t, createWorker, "state-path", "socket-path", "session-index", "space", "name", "peer-id", "cluster")
}
