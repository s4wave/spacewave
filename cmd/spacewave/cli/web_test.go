//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

func TestBuildWebListenMultiaddr(t *testing.T) {
	tests := []struct {
		host string
		port uint32
		want string
	}{
		{host: "127.0.0.1", port: 0, want: "/ip4/127.0.0.1/tcp/0"},
		{host: "::1", port: 8080, want: "/ip6/::1/tcp/8080"},
		{host: "[::1]", port: 8080, want: "/ip6/::1/tcp/8080"},
		{host: "localhost", port: 0, want: "/dns4/localhost/tcp/0"},
	}
	for _, tt := range tests {
		got := buildWebListenMultiaddr(tt.host, tt.port)
		if got != tt.want {
			t.Fatalf("buildWebListenMultiaddr(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
		}
	}
}

func TestBackgroundWebListenerSurvivesClientDisconnectPastIdle(t *testing.T) {
	// Cancel the test context when the test ends.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Register a core root server.
	le := logrus.NewEntry(logrus.New())
	rootMux := srpc.NewMux()
	rootServer := resource_root.NewCoreRootServer(le, nil)
	defer rootServer.Close()
	if err := rootServer.Register(rootMux); err != nil {
		t.Fatal(err)
	}

	// Register the resource server on that mux.
	resourceSrv := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := resourceSrv.Register(resourceMux); err != nil {
		t.Fatal(err)
	}

	// Build a short idle tracker.
	idleCh := make(chan struct{}, 1)
	idleTracker := newDaemonIdleTracker(30*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer idleTracker.close()

	// Start the web listener keepalive.
	startWebListenerKeepalive(ctx, le, resourceMux, idleTracker)

	// Pipe a tracked daemon connection.
	daemonMux := srpc.NewMux(resourceMux)
	server := srpc.NewServer(daemonMux)
	clientConn, serverConn := net.Pipe()

	// Attach the client and accept the muxed connection.
	idleTracker.clientAttached()
	tracked := &trackedConn{
		Conn: serverConn,
		onClose: func() {
			idleTracker.clientDetached()
		},
	}
	serverMp, err := srpc.NewMuxedConn(tracked, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Access a background web listener.
	client, err := buildSDKClient(ctx, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.root.AccessWebListener(ctx, "/ip4/127.0.0.1/tcp/0", true)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the keepalive to hold the listener before the client leaves.
	waitIdleTrackerActive(t, ctx, idleTracker, 2)
	client.close()

	// Require idle not to fire while the listener is active.
	select {
	case <-idleCh:
		t.Fatal("daemon idle fired while background listener was active")
	case <-time.After(100 * time.Millisecond):
	}

	// Require the health endpoint to answer after the client leaves.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resp.GetUrl()+"/_spacewave/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if httpResp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("health response = %d %q, want ok", httpResp.StatusCode, string(body))
	}

	// Require idle after the root server closes.
	rootServer.Close()
	select {
	case <-idleCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected daemon idle after background listener close")
	}
}

// waitIdleTrackerActive waits until the tracker counts want active holds.
func waitIdleTrackerActive(t *testing.T, ctx context.Context, tracker *daemonIdleTracker, want int) {
	t.Helper()

	for {
		tracker.mu.Lock()
		active := tracker.active
		tracker.mu.Unlock()
		if active == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("idle tracker active = %d, want %d", active, want)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestGlobalStatePathWebBackgroundAndFollowupUseSameSocket(t *testing.T) {
	// Cancel the test context when the test ends.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start an in-process web daemon.
	daemon := startInProcessWebDaemon(t, ctx)
	statePath := daemon.statePath
	accepted := daemon.accepted

	// Start a background listener, list it, and stop it.
	runStatePathWebApp(t, ctx, []string{
		"--state-path", statePath,
		"web",
		"--background",
	})
	listeners := getWebListeners(t, ctx, statePath)
	if len(listeners) != 1 {
		t.Fatalf("listeners = %d, want 1", len(listeners))
	}
	listenerID := listeners[0].GetListenerId()
	runStatePathWebApp(t, ctx, []string{
		"--state-path", statePath,
		"web",
		"list",
	})
	runStatePathWebApp(t, ctx, []string{
		"--state-path", statePath,
		"web",
		"stop",
		listenerID,
	})

	// Require each command to connect to the daemon socket.
	for range 3 {
		select {
		case <-accepted:
		case <-time.After(time.Second):
			t.Fatal("expected command to connect to configured daemon socket")
		}
	}

	// Require the listener to be gone after stop.
	listeners = getWebListeners(t, ctx, statePath)
	if len(listeners) != 0 {
		t.Fatalf("listeners after stop = %d, want 0", len(listeners))
	}
}

func TestWebBackgroundPrintURLWritesMachineReadableURL(t *testing.T) {
	// Cancel the test context when the test ends.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start an in-process web daemon.
	daemon := startInProcessWebDaemon(t, ctx)
	statePath := daemon.statePath

	// Capture the printed URL and require one line.
	stdout, stderr := runStatePathWebAppCapture(t, ctx, []string{
		"--state-path", statePath,
		"web",
		"--background",
		"--print-url",
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.HasSuffix(stdout, "\n") || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one URL line", stdout)
	}

	// Parse the URL and require an HTTP root.
	urlLine := strings.TrimSuffix(stdout, "\n")
	parsedURL, err := url.Parse(urlLine)
	if err != nil {
		t.Fatalf("stdout URL = %q, parse: %v", urlLine, err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		t.Fatalf("stdout URL = %q, want http or https URL", urlLine)
	}
	if parsedURL.Path != "/" {
		t.Fatalf("stdout URL path = %q, want /", parsedURL.Path)
	}
	if parsedURL.RawQuery != "" {
		t.Fatalf("stdout URL query = %q, want empty", parsedURL.RawQuery)
	}

	// Require a non-empty OTP and no secret in the remaining text.
	fragmentKey, secret, ok := strings.Cut(parsedURL.Fragment, "=")
	if !ok || fragmentKey != "otp" {
		t.Fatalf("stdout URL fragment = %q, want otp secret", parsedURL.Fragment)
	}
	if secret == "" {
		t.Fatalf("stdout URL = %q, want non-empty OTP secret", urlLine)
	}
	stdoutWithoutSecret := strings.Replace(stdout, secret, "<secret>", 1)
	for _, text := range []string{
		"Spacewave is running",
		"Reusing background",
		"Use",
		"Press Ctrl-C",
	} {
		if strings.Contains(stdoutWithoutSecret, text) {
			t.Fatalf("stdout = %q, must not contain banner text %q", stdout, text)
		}
	}
}

func TestWebBackgroundPrintURLWritesDisplayURLBeforeOTPFragment(t *testing.T) {
	// Cancel the test context when the test ends.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start the daemon and choose a display path and component.
	daemon := startInProcessWebDaemon(t, ctx)
	statePath := daemon.statePath
	displayPath := "docs/hello world/-/child.txt"
	displayComponent := "viewer.markdown/primary"

	// Capture the display URL and require one line.
	stdout, stderr := runStatePathWebAppCapture(t, ctx, []string{
		"--state-path", statePath,
		"web",
		"--background",
		"--print-url",
		"--display", displayPath,
		"--display-component", displayComponent,
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.HasSuffix(stdout, "\n") || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one URL line", stdout)
	}

	// Require the display path and component query.
	urlLine := strings.TrimSuffix(stdout, "\n")
	parsedURL, err := url.Parse(urlLine)
	if err != nil {
		t.Fatalf("stdout URL = %q, parse: %v", urlLine, err)
	}
	if parsedURL.Path != "/display" {
		t.Fatalf("stdout URL path = %q, want /display", parsedURL.Path)
	}
	query := parsedURL.Query()
	if query.Get("path") != displayPath {
		t.Fatalf("display path query = %q, want %q", query.Get("path"), displayPath)
	}
	if query.Get("component") != displayComponent {
		t.Fatalf("display component query = %q, want %q", query.Get("component"), displayComponent)
	}

	// Require a non-empty OTP fragment.
	fragmentKey, secret, ok := strings.Cut(parsedURL.Fragment, "=")
	if !ok || fragmentKey != "otp" {
		t.Fatalf("stdout URL fragment = %q, want otp secret", parsedURL.Fragment)
	}
	if secret == "" {
		t.Fatalf("stdout URL = %q, want non-empty OTP secret", urlLine)
	}
}

func TestWebBackgroundPrintURLWritesDisplayPathWithoutComponent(t *testing.T) {
	// Cancel the test context when the test ends.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start the daemon and choose a display path.
	daemon := startInProcessWebDaemon(t, ctx)
	statePath := daemon.statePath
	displayPath := "docs/hello"

	// Capture the display URL.
	stdout, stderr := runStatePathWebAppCapture(t, ctx, []string{
		"--state-path", statePath,
		"web",
		"--background",
		"--print-url",
		"--display", displayPath,
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	// Require the display path and no component query.
	urlLine := strings.TrimSuffix(stdout, "\n")
	parsedURL, err := url.Parse(urlLine)
	if err != nil {
		t.Fatalf("stdout URL = %q, parse: %v", urlLine, err)
	}
	if parsedURL.Path != "/display" {
		t.Fatalf("stdout URL path = %q, want /display", parsedURL.Path)
	}
	query := parsedURL.Query()
	if query.Get("path") != displayPath {
		t.Fatalf("display path query = %q, want %q", query.Get("path"), displayPath)
	}
	if query.Has("component") {
		t.Fatalf("display component query = %q, want omitted", query.Get("component"))
	}
}

func TestWebDisplayComponentRequiresDisplayPath(t *testing.T) {
	// Run web with a display component and no display path.
	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{
		newWebCommand(nil),
	}
	err := app.RunContext(t.Context(), []string{
		"spacewave",
		"web",
		"--background",
		"--print-url",
		"--display-component",
		"viewer.markdown",
	})
	if err == nil || !strings.Contains(err.Error(), "--display-component requires --display") {
		t.Fatalf("error = %v, want --display-component requires --display", err)
	}
}

type testWebDaemon struct {
	statePath string
	accepted  <-chan struct{}
}

func startInProcessWebDaemon(t *testing.T, ctx context.Context) testWebDaemon {
	// Mark the helper.
	t.Helper()

	// Clear the state-path and socket-path environment.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	// Create a short state directory.
	statePath := shortSocketDir(t)

	// Register a core root server.
	le := logrus.NewEntry(logrus.New())
	rootMux := srpc.NewMux()
	rootServer := resource_root.NewCoreRootServer(le, nil)
	t.Cleanup(rootServer.Close)
	if err := rootServer.Register(rootMux); err != nil {
		t.Fatal(err)
	}

	// Register the resource server.
	resourceSrv := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := resourceSrv.Register(resourceMux); err != nil {
		t.Fatal(err)
	}

	// Build a long idle tracker.
	idleTracker := newDaemonIdleTracker(time.Minute, func() {})
	t.Cleanup(idleTracker.close)

	// Listen on the daemon socket.
	lis, err := net.Listen("unix", filepath.Join(statePath, socketName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = lis.Close()
	})

	// Accept connections and count them.
	accepted := make(chan struct{}, 8)
	server := srpc.NewServer(srpc.NewMux(resourceMux))
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			idleTracker.clientAttached()
			go func() {
				defer idleTracker.clientDetached()
				mp, err := srpc.NewMuxedConn(conn, false, nil)
				if err != nil {
					conn.Close()
					return
				}
				_ = server.AcceptMuxedConn(ctx, mp)
			}()
		}
	}()

	// Reject daemon autostart.
	oldStart := connectDaemonStart
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		return stderrors.New("unexpected daemon autostart")
	}
	t.Cleanup(func() {
		connectDaemonStart = oldStart
	})

	return testWebDaemon{
		statePath: statePath,
		accepted:  accepted,
	}
}

func getWebListeners(t *testing.T, ctx context.Context, statePath string) []*s4wave_root.WebListenerInfo {
	// Mark the helper.
	t.Helper()

	// List web listeners from the daemon.
	client, err := connectDaemon(ctx, statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	listeners, err := client.root.ListWebListeners(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return listeners
}

func runStatePathWebApp(t *testing.T, ctx context.Context, args []string) {
	// Mark the helper.
	t.Helper()

	// Run the web app with the given arguments.
	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{
		newWebCommand(nil),
	}
	if err := app.RunContext(ctx, append([]string{"spacewave"}, args...)); err != nil {
		t.Fatal(err)
	}
}

func runStatePathWebAppCapture(t *testing.T, ctx context.Context, args []string) (string, string) {
	// Mark the helper.
	t.Helper()

	// Build the web app.
	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{
		newWebCommand(nil),
	}

	// Run it and return the captured streams.
	stdout, stderr, err := captureStdoutStderr(t, func() error {
		return app.RunContext(ctx, append([]string{"spacewave"}, args...))
	})
	if err != nil {
		t.Fatal(err)
	}
	return stdout, stderr
}

func captureStdoutStderr(t *testing.T, fn func() error) (string, string, error) {
	// Mark the helper.
	t.Helper()

	// Open stdout and stderr pipes.
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	// Point stdout and stderr at the pipes.
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	// Run the function and close the writers.
	runErr := fn()
	if err := stdoutW.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := stderrW.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}

	// Read both streams and close the readers.
	stdout, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderr, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := stdoutR.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	if err := stderrR.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return string(stdout), string(stderr), runErr
}
