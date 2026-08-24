//go:build !js

package spacewave_cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
)

// TestResolveStatePathRequiresExplicitStatePath asserts that attaching to a
// daemon refuses the shared default state root (~/.spacewave) when the user
// gave no --state-path flag, state path environment variable, or live
// project-local daemon. The shared default may hold the operator's live
// world; an implicit attach has mounted every Space in it.
func TestResolveStatePathRequiresExplicitStatePath(t *testing.T) {
	clearStatePathEnv(t)

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	chdir(t, t.TempDir())

	oldDefault := defaultStatePath
	defaultStatePath = filepath.Join(tempHome, ".spacewave")
	t.Cleanup(func() { defaultStatePath = oldDefault })

	var got string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Commands = []*cli.Command{{
		Name: "check",
		Action: func(c *cli.Context) error {
			resolved, err := resolveStatePathFromContext(c, "")
			if err != nil {
				return err
			}
			got = resolved
			return nil
		},
	}}
	err := app.RunContext(context.Background(), []string{"spacewave", "check"})
	if err == nil {
		t.Fatalf("implicit default state path accepted: resolved %s", got)
	}
	if !strings.Contains(err.Error(), defaultStatePath) && !strings.Contains(err.Error(), "--state-path") {
		t.Fatalf("error does not name the default or the explicit flag: %v", err)
	}
}

// TestResolveStatePathAcceptsExplicitEnv asserts the state path environment
// variable still resolves without error after the implicit-default guard.
func TestResolveStatePathAcceptsExplicitEnv(t *testing.T) {
	clearStatePathEnv(t)

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	chdir(t, t.TempDir())

	oldDefault := defaultStatePath
	defaultStatePath = filepath.Join(tempHome, ".spacewave")
	t.Cleanup(func() { defaultStatePath = oldDefault })

	statePath := filepath.Join(t.TempDir(), "explicit")
	t.Setenv(statePathEnvVars[0], statePath)

	var rootStatePath string
	var got string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{{
		Name: "check",
		Action: func(c *cli.Context) error {
			resolved, err := resolveStatePathFromContext(c, "")
			if err != nil {
				return err
			}
			got = resolved
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), []string{"spacewave", "check"}); err != nil {
		t.Fatal(err)
	}
	if got != statePath {
		t.Fatalf("got %s, want %s", got, statePath)
	}
}
