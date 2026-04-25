//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestResolveStatePathFromContextUsesGlobalFlag(t *testing.T) {
	clearStatePathEnv(t)

	statePath := filepath.Join(t.TempDir(), "global")
	got := runStatePathResolveCommand(t, []string{"--state-path", statePath, "check"})
	if got != statePath {
		t.Fatalf("got %s, want %s", got, statePath)
	}
}

// TestResolveStatePathFromContextWithClientFlags asserts that subcommands
// registering --state-path through clientFlags still receive the global
// --state-path value when only the global flag is set. This guards against
// the regression where commands called resolveStatePath(statePath) directly
// and silently used the local flag's default (defaultStatePath), ignoring
// the global flag passed before the subcommand name.
func TestResolveStatePathFromContextWithClientFlags(t *testing.T) {
	clearStatePathEnv(t)

	statePath := filepath.Join(t.TempDir(), "global")

	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string
	var got string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{{
		Name:  "check",
		Flags: clientFlags(&commandStatePath, &commandSessionIdx),
		Action: func(c *cli.Context) error {
			resolved, err := resolveStatePathFromContext(c, commandStatePath)
			if err != nil {
				return err
			}
			got = resolved
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), []string{"spacewave", "--state-path", statePath, "check"}); err != nil {
		t.Fatal(err)
	}
	if got != statePath {
		t.Fatalf("got %s, want %s", got, statePath)
	}
}

func TestResolveStatePathFromContextUsesCommandRelativeFlag(t *testing.T) {
	clearStatePathEnv(t)

	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	got := runStatePathResolveCommand(t, []string{"check", "--state-path", "state"})
	want := filepath.Join(cwd, "state")
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestResolveStatePathFromContextUsesEnv(t *testing.T) {
	clearStatePathEnv(t)

	statePath := filepath.Join(t.TempDir(), "env")
	if err := os.Setenv(statePathEnvVars[0], statePath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(statePathEnvVars[0])
	})

	got := runStatePathResolveCommand(t, []string{"check"})
	if got != statePath {
		t.Fatalf("got %s, want %s", got, statePath)
	}
}

func TestResolveStatePathFromContextUsesDefault(t *testing.T) {
	clearStatePathEnv(t)

	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	got := runStatePathResolveCommand(t, []string{"check"})
	want := defaultStatePath
	if !filepath.IsAbs(want) {
		want = filepath.Join(cwd, want)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestResolveStatePathPrefersExistingCwdSocket(t *testing.T) {
	clearStatePathEnv(t)

	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(cwd, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, socketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveStatePath("state")
	if err != nil {
		t.Fatal(err)
	}
	if got != stateDir {
		t.Fatalf("got %s, want %s", got, stateDir)
	}
}

func TestResolveStatePathFallsBackToGitRootSocket(t *testing.T) {
	clearStatePathEnv(t)

	root := t.TempDir()
	cmd := exec.Command("git", "init", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, socketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, subdir)

	got, err := resolveStatePath("state")
	if err != nil {
		t.Fatal(err)
	}
	if got != stateDir {
		t.Fatalf("got %s, want %s", got, stateDir)
	}
}

func runStatePathResolveCommand(t *testing.T, args []string) string {
	t.Helper()

	var commandStatePath string
	var rootStatePath string
	var got string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{{
		Name: "check",
		Flags: []cli.Flag{
			statePathFlag(&commandStatePath),
		},
		Action: func(c *cli.Context) error {
			resolved, err := resolveStatePathFromContext(c, commandStatePath)
			if err != nil {
				return err
			}
			got = resolved
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), append([]string{"spacewave"}, args...)); err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("state path was not resolved")
	}
	return got
}

func clearStatePathEnv(t *testing.T) {
	t.Helper()

	for _, name := range statePathEnvVars {
		value, ok := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(name, value)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}

func chdir(t *testing.T, path string) {
	t.Helper()

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}
