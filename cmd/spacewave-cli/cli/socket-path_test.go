//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
)

// TestEffectiveSocketPathUsesCommandFlag asserts the command-level
// --socket-path flag is returned through the lineage walk when set
// directly on a subcommand context.
func TestEffectiveSocketPathUsesCommandFlag(t *testing.T) {
	clearSocketPathEnv(t)

	sock := filepath.Join(t.TempDir(), "desktop.sock")
	got := runSocketPathResolveCommand(t, []string{"check", "--socket-path", sock})
	if got != sock {
		t.Fatalf("got %s, want %s", got, sock)
	}
}

// TestEffectiveSocketPathUsesEnv asserts SPACEWAVE_SOCKET_PATH is
// picked up as an env fallback for the flag.
func TestEffectiveSocketPathUsesEnv(t *testing.T) {
	clearSocketPathEnv(t)

	sock := filepath.Join(t.TempDir(), "desktop.sock")
	t.Setenv(socketPathEnvVars[0], sock)

	got := runSocketPathResolveCommand(t, []string{"check"})
	if got != sock {
		t.Fatalf("got %s, want %s", got, sock)
	}
}

// TestEffectiveSocketPathFlagBeatsEnv asserts an explicit --socket-path
// flag value takes precedence over SPACEWAVE_SOCKET_PATH.
func TestEffectiveSocketPathFlagBeatsEnv(t *testing.T) {
	clearSocketPathEnv(t)

	envSock := filepath.Join(t.TempDir(), "env.sock")
	flagSock := filepath.Join(t.TempDir(), "flag.sock")
	t.Setenv(socketPathEnvVars[0], envSock)

	got := runSocketPathResolveCommand(t, []string{"check", "--socket-path", flagSock})
	if got != flagSock {
		t.Fatalf("got %s, want %s", got, flagSock)
	}
}

// TestEffectiveSocketPathUnsetReturnsFallback asserts absent flag and
// env yield the caller-provided fallback (empty by convention).
func TestEffectiveSocketPathUnsetReturnsFallback(t *testing.T) {
	clearSocketPathEnv(t)

	got := runSocketPathResolveCommand(t, []string{"check"})
	if got != "" {
		t.Fatalf("got %s, want empty", got)
	}
}

// TestConnectDaemonAtSocketSkipsAutostart asserts connect-only mode
// never invokes the daemon autostart path, even on dial failure.
func TestConnectDaemonAtSocketSkipsAutostart(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		return nil, context.DeadlineExceeded
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("autostart must not run in connect-only mode")
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		t.Fatal("build client must not run after dial failure")
		return nil, nil
	}

	_, err := connectDaemonAtSocket(context.Background(), "/tmp/desktop.sock")
	if err == nil {
		t.Fatal("expected dial failure error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "/tmp/desktop.sock") {
		t.Fatalf("expected error to name socket path: %v", err)
	}
	if !strings.Contains(msg, "Spacewave desktop app") || !strings.Contains(msg, "spacewave serve") {
		t.Fatalf("expected actionable guidance, got: %v", err)
	}
}

// TestConnectDaemonFromContextUsesSocketPath asserts --socket-path on
// a command context routes to the connect-only dial path and never
// resolves state-path.
func TestConnectDaemonFromContextUsesSocketPath(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	sock := filepath.Join(t.TempDir(), "desktop.sock")
	var dialedSocket string
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialedSocket = sockPath
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("autostart must not run when --socket-path is set")
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{{
		Name:  "check",
		Flags: clientFlags(&commandStatePath, &commandSessionIdx),
		Action: func(c *cli.Context) error {
			client, err := connectDaemonFromContext(c.Context, c, commandStatePath)
			if err != nil {
				return err
			}
			client.conn.Close()
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), []string{"spacewave", "check", "--socket-path", sock}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if dialedSocket != sock {
		t.Fatalf("dialed %s, want %s", dialedSocket, sock)
	}
}

// TestConnectDaemonFromContextFallsBackToStatePath asserts no
// --socket-path falls through to the state-path resolve + autostart
// flow.
func TestConnectDaemonFromContextFallsBackToStatePath(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	var dialedSocket string
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialedSocket = sockPath
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	statePath := filepath.Join(t.TempDir(), "state")
	want := filepath.Join(statePath, socketName)

	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{{
		Name:  "check",
		Flags: clientFlags(&commandStatePath, &commandSessionIdx),
		Action: func(c *cli.Context) error {
			client, err := connectDaemonFromContext(c.Context, c, commandStatePath)
			if err != nil {
				return err
			}
			client.conn.Close()
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), []string{"spacewave", "--state-path", statePath, "check"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if dialedSocket != want {
		t.Fatalf("dialed %s, want %s", dialedSocket, want)
	}
}

// TestDiscoverProjectLocalStatePathUsesCwd asserts cwd/.spacewave
// with a live socket wins over the shared default root.
func TestDiscoverProjectLocalStatePathUsesCwd(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(cwd, projectLocalStateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, socketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := discoverProjectLocalStatePath()
	if !ok {
		t.Fatal("expected discovery to find cwd socket")
	}
	if got != stateDir {
		t.Fatalf("got %s, want %s", got, stateDir)
	}
}

// TestDiscoverProjectLocalStatePathMissing asserts the function
// reports no match when no local socket is present.
func TestDiscoverProjectLocalStatePathMissing(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	chdir(t, t.TempDir())

	got, ok := discoverProjectLocalStatePath()
	if ok {
		t.Fatalf("expected no discovery, got %s", got)
	}
}

// TestResolveStatePathFromContextPrefersProjectLocalOverDefault asserts
// that when --state-path is unset, a project-local socket wins over
// the shared default root (~/.spacewave).
func TestResolveStatePathFromContextPrefersProjectLocalOverDefault(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(cwd, projectLocalStateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, socketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got := runStatePathResolveCommand(t, []string{"check"})
	if got != stateDir {
		t.Fatalf("got %s, want %s", got, stateDir)
	}
}

// TestResolveStatePathFromContextExplicitFlagSkipsDiscovery asserts
// that when --state-path is explicitly set, project-local discovery
// is skipped even if a local socket would have matched.
func TestResolveStatePathFromContextExplicitFlagSkipsDiscovery(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// A cwd/.spacewave/spacewave.sock exists but must be ignored when
	// --state-path is set explicitly.
	localDir := filepath.Join(cwd, projectLocalStateDirName)
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, socketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	explicit := filepath.Join(t.TempDir(), "explicit")
	got := runStatePathResolveCommand(t, []string{"--state-path", explicit, "check"})
	if got != explicit {
		t.Fatalf("got %s, want %s", got, explicit)
	}
}

// runSocketPathResolveCommand runs a minimal cli app wired with
// clientFlags and returns the resolved socket path reported by
// effectiveSocketPath.
func runSocketPathResolveCommand(t *testing.T, args []string) string {
	t.Helper()

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
			got = effectiveSocketPath(c, "")
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), append([]string{"spacewave"}, args...)); err != nil {
		t.Fatal(err)
	}
	return got
}

// clearSocketPathEnv unsets SPACEWAVE_SOCKET_PATH for the test and
// restores the prior value on cleanup.
func clearSocketPathEnv(t *testing.T) {
	t.Helper()

	for _, name := range socketPathEnvVars {
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
