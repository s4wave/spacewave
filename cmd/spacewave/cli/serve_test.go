//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
)

func TestServeCommandTraceFlag(t *testing.T) {
	cmd := newServeCommand(nil)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if set.Lookup("trace") == nil {
		t.Fatal("trace flag missing")
	}
}

func TestServeCommandIdleTimeoutFlag(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "")
	cmd := newServeCommand(nil)
	idleFlag := findServeIdleTimeoutFlag(t, cmd)

	if idleFlag.Value != defaultDaemonIdleTimeout {
		t.Fatalf("idle-timeout default = %v, want %v", idleFlag.Value, defaultDaemonIdleTimeout)
	}
	if idleFlag.GetDefaultText() != defaultDaemonIdleTimeout.String() {
		t.Fatalf("idle-timeout default text = %q, want %q", idleFlag.GetDefaultText(), defaultDaemonIdleTimeout)
	}
	if !strings.Contains(idleFlag.Usage, "last active client/service") ||
		!strings.Contains(idleFlag.Usage, "zero disables idle shutdown") {
		t.Fatalf("idle-timeout usage does not describe shutdown behavior: %q", idleFlag.Usage)
	}
}

func TestServeCommandIdleTimeoutFlagUsesEnvironment(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "45s")
	cmd := newServeCommand(nil)
	idleFlag := findServeIdleTimeoutFlag(t, cmd)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if err := idleFlag.Apply(set); err != nil {
		t.Fatalf("apply idle-timeout flag: %v", err)
	}

	if idleFlag.Value != 45*time.Second {
		t.Fatalf("idle-timeout environment value = %v, want %v", idleFlag.Value, 45*time.Second)
	}
}

func TestServeCommandIdleTimeoutFlagOverridesEnvironment(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "45s")
	cmd := newServeCommand(nil)
	idleFlag := findServeIdleTimeoutFlag(t, cmd)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if err := set.Parse([]string{"--idle-timeout", "0s"}); err != nil {
		t.Fatalf("parse idle-timeout flag: %v", err)
	}
	if idleFlag.Destination == nil {
		t.Fatal("idle-timeout flag destination missing")
	}
	if *idleFlag.Destination != 0 {
		t.Fatalf("idle-timeout destination = %v, want 0", *idleFlag.Destination)
	}
}

func findServeIdleTimeoutFlag(t *testing.T, cmd *cli.Command) *cli.DurationFlag {
	t.Helper()
	for _, fl := range cmd.Flags {
		idleFlag, ok := fl.(*cli.DurationFlag)
		if ok && idleFlag.Name == "idle-timeout" {
			return idleFlag
		}
	}
	t.Fatal("idle-timeout flag missing")
	return nil
}
