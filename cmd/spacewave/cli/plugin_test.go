//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestPluginSubcommandsUseClientFlags(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cli.Command
	}{
		{"list", buildPluginListCommand()},
		{"add", buildPluginAddCommand()},
		{"remove", buildPluginRemoveCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := flag.NewFlagSet(tt.name, flag.ContinueOnError)
			set.SetOutput(io.Discard)
			for _, fl := range tt.cmd.Flags {
				if err := fl.Apply(set); err != nil {
					t.Fatalf("apply flag: %v", err)
				}
			}
			for _, name := range []string{"state-path", "socket-path", "session-index", "space"} {
				if set.Lookup(name) == nil {
					t.Fatalf("%s flag missing", name)
				}
			}
		})
	}
}

func TestPluginImportManifestObjectKeyFlag(t *testing.T) {
	cmd := buildPluginImportManifestCommand(nil)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	objectKey := set.Lookup("object-key")
	if objectKey == nil {
		t.Fatal("object-key flag missing")
	}
	if objectKey.DefValue != daemonPluginHostObjectKey {
		t.Fatalf("object-key default = %q, want %q", objectKey.DefValue, daemonPluginHostObjectKey)
	}
	if set.Lookup("target-db") == nil {
		t.Fatal("target-db flag missing")
	}
}
