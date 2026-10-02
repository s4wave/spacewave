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
	// Clear the socket-path environment.
	clearSocketPathEnv(t)

	// Set the socket-path environment variable.
	sock := filepath.Join(t.TempDir(), "desktop.sock")
	t.Setenv(socketPathEnvVars[0], sock)

	// Require the environment socket path.
	got := runSocketPathResolveCommand(t, []string{"check"})
	if got != sock {
		t.Fatalf("got %s, want %s", got, sock)
	}
}

// TestEffectiveSocketPathFlagBeatsEnv asserts an explicit --socket-path
// flag value takes precedence over SPACEWAVE_SOCKET_PATH.
func TestEffectiveSocketPathFlagBeatsEnv(t *testing.T) {
	// Clear the socket-path environment.
	clearSocketPathEnv(t)

	// Set an environment socket and a flag socket.
	envSock := filepath.Join(t.TempDir(), "env.sock")
	flagSock := filepath.Join(t.TempDir(), "flag.sock")
	t.Setenv(socketPathEnvVars[0], envSock)

	// Require the flag socket to win.
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

// TestServeSocketPathUsesExplicitListener proves serve does not replace an
// exact socket path with the state-local default.
func TestServeSocketPathUsesExplicitListener(t *testing.T) {
	// Clear the socket-path environment and choose an explicit socket.
	clearSocketPathEnv(t)
	statePath := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "device.sock")

	// Declare the captured socket path.
	var rootStatePath string
	var got string

	// Build the app and run check with the explicit socket.
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath), socketPathFlag()}
	app.Commands = []*cli.Command{{
		Name: "check",
		Action: func(c *cli.Context) error {
			got = serveSocketPath(c, statePath)
			return nil
		},
	}}
	if err := app.RunContext(context.Background(), []string{"spacewave", "--socket-path", explicit, "check"}); err != nil {
		t.Fatal(err)
	}

	// Require the serve socket to match the explicit path.
	if got != explicit {
		t.Fatalf("serve socket = %q, want %q", got, explicit)
	}
}

// TestConnectDaemonAtSocketSkipsAutostart asserts connect-only mode
// never invokes the daemon autostart path, even on dial failure.
func TestConnectDaemonAtSocketSkipsAutostart(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Fail dial and forbid autostart and client build.
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

	// Require the dial error to name the socket and the desktop app.
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
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Pipe a connection and close it when the test ends.
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Dial the explicit socket and skip autostart.
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

	// Declare the command state-path flags.
	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string

	// Build the app with a check command.
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

	// Run check with the socket-path flag.
	if err := app.RunContext(context.Background(), []string{"spacewave", "check", "--socket-path", sock}); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Require the dialed socket to match.
	if dialedSocket != sock {
		t.Fatalf("dialed %s, want %s", dialedSocket, sock)
	}
}

// TestConnectDaemonFromContextFallsBackToStatePath asserts no --socket-path
// uses the state-path daemon socket.
func TestConnectDaemonFromContextFallsBackToStatePath(t *testing.T) {
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Pipe a connection and close it when the test ends.
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Dial successfully and skip daemon start.
	var dialedSocket string
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialedSocket = sockPath
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("daemon start must not run when dial succeeds")
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	// Choose a state-path socket.
	statePath := filepath.Join(t.TempDir(), "state")
	want := filepath.Join(statePath, socketName)

	// Declare the command state-path flags.
	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string

	// Build the app with a check command.
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

	// Run check with the state-path flag.
	if err := app.RunContext(context.Background(), []string{"spacewave", "--state-path", statePath, "check"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Require the dialed socket to match the state-path socket.
	if dialedSocket != want {
		t.Fatalf("dialed %s, want %s", dialedSocket, want)
	}
}

// TestConnectDaemonFromContextStartsStatePathDaemon asserts state-path commands
// launch a daemon after an initial dial failure.
func TestConnectDaemonFromContextStartsStatePathDaemon(t *testing.T) {
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Pipe a connection and close it when the test ends.
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Fail the first dial, then start the daemon.
	var dialCalls int
	var startStatePath string
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCalls++
		if dialCalls == 1 {
			return nil, os.ErrNotExist
		}
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		startStatePath = statePath
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	// Choose a state path.
	statePath := filepath.Join(t.TempDir(), "state")

	// Declare the command state-path flags.
	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string

	// Build the app with a check command.
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

	// Run check with the state-path flag.
	if err := app.RunContext(context.Background(), []string{"spacewave", "--state-path", statePath, "check"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Require the daemon to start at that state path and dial twice.
	if startStatePath != statePath {
		t.Fatalf("started with %s, want %s", startStatePath, statePath)
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial attempts, got %d", dialCalls)
	}
}

// TestDiscoverProjectLocalStatePathUsesCwd asserts cwd/.spacewave
// with a live socket wins over the shared default root.
func TestDiscoverProjectLocalStatePathUsesCwd(t *testing.T) {
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Change into a temporary working directory.
	cwd := t.TempDir()
	chdir(t, cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Create a project-local state directory with a socket.
	stateDir := filepath.Join(cwd, projectLocalStateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, socketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Require discovery to return that directory.
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
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Change into an empty temporary directory.
	chdir(t, t.TempDir())

	// Require discovery to find no socket.
	got, ok := discoverProjectLocalStatePath()
	if ok {
		t.Fatalf("expected no discovery, got %s", got)
	}
}

// TestResolveStatePathFromContextPrefersProjectLocalOverDefault asserts
// that when --state-path is unset, a project-local socket wins over
// the shared default root (~/.spacewave).
func TestResolveStatePathFromContextPrefersProjectLocalOverDefault(t *testing.T) {
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Create a project-local socket in the working directory.
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

	// Require that directory over the default state path.
	got := runStatePathResolveCommand(t, []string{"check"})
	if got != stateDir {
		t.Fatalf("got %s, want %s", got, stateDir)
	}
}

// TestResolveStatePathFromContextExplicitFlagSkipsDiscovery asserts
// that when --state-path is explicitly set, project-local discovery
// is skipped even if a local socket would have matched.
func TestResolveStatePathFromContextExplicitFlagSkipsDiscovery(t *testing.T) {
	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Change into a temporary working directory.
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

	// Require the explicit flag to skip the project-local socket.
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
	// Mark the helper.
	t.Helper()

	// Declare the captured paths.
	var commandStatePath string
	var commandSessionIdx uint
	var rootStatePath string
	var got string

	// Build the app with a check command.
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

	// Run check and return the resolved socket.
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
