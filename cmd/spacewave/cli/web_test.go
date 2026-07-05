//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	rootMux := srpc.NewMux()
	rootServer := resource_root.NewCoreRootServer(le, nil)
	defer rootServer.Close()
	if err := rootServer.Register(rootMux); err != nil {
		t.Fatal(err)
	}

	resourceSrv := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := resourceSrv.Register(resourceMux); err != nil {
		t.Fatal(err)
	}

	idleCh := make(chan struct{}, 1)
	idleTracker := newDaemonIdleTracker(30*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer idleTracker.close()

	resetKeepalive := resource_root.SetWebListenerKeepaliveFunc(func(listenerID string) func() {
		if listenerID == "" {
			t.Fatal("listener id should be set before keepalive")
		}
		return idleTracker.serviceAttached()
	})
	defer resetKeepalive()

	daemonMux := srpc.NewMux(resourceMux)
	server := srpc.NewServer(daemonMux)
	clientConn, serverConn := net.Pipe()

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

	client, err := buildSDKClient(ctx, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.root.AccessWebListener(ctx, "/ip4/127.0.0.1/tcp/0", true)
	if err != nil {
		t.Fatal(err)
	}
	client.close()

	select {
	case <-idleCh:
		t.Fatal("daemon idle fired while background listener was active")
	case <-time.After(100 * time.Millisecond):
	}

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

	rootServer.Close()
	select {
	case <-idleCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected daemon idle after background listener close")
	}
}

func TestGlobalStatePathWebBackgroundAndFollowupUseSameSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	daemon := startInProcessWebDaemon(t, ctx)
	statePath := daemon.statePath
	accepted := daemon.accepted

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

	for range 3 {
		select {
		case <-accepted:
		case <-time.After(time.Second):
			t.Fatal("expected command to connect to configured daemon socket")
		}
	}

	listeners = getWebListeners(t, ctx, statePath)
	if len(listeners) != 0 {
		t.Fatalf("listeners after stop = %d, want 0", len(listeners))
	}
}

func TestWebBackgroundPrintURLWritesMachineReadableURL(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	daemon := startInProcessWebDaemon(t, ctx)
	statePath := daemon.statePath

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
	urlLine := strings.TrimSuffix(stdout, "\n")
	if !strings.HasPrefix(urlLine, "http://") && !strings.HasPrefix(urlLine, "https://") {
		t.Fatalf("stdout URL = %q, want http or https URL", urlLine)
	}
	_, secret, ok := strings.Cut(urlLine, "/#otp=")
	if !ok {
		t.Fatalf("stdout URL = %q, want OTP fragment", urlLine)
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

type testWebDaemon struct {
	statePath string
	accepted  <-chan struct{}
}

func startInProcessWebDaemon(t *testing.T, ctx context.Context) testWebDaemon {
	t.Helper()

	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	tmpRoot, err := filepath.Abs(".tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath, err := os.MkdirTemp(tmpRoot, "web-state-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(statePath)
	})

	le := logrus.NewEntry(logrus.New())
	rootMux := srpc.NewMux()
	rootServer := resource_root.NewCoreRootServer(le, nil)
	t.Cleanup(rootServer.Close)
	if err := rootServer.Register(rootMux); err != nil {
		t.Fatal(err)
	}

	resourceSrv := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := resourceSrv.Register(resourceMux); err != nil {
		t.Fatal(err)
	}

	idleTracker := newDaemonIdleTracker(time.Minute, func() {})
	t.Cleanup(idleTracker.close)

	resetKeepalive := resource_root.SetWebListenerKeepaliveFunc(func(listenerID string) func() {
		return idleTracker.serviceAttached()
	})
	t.Cleanup(resetKeepalive)

	lis, err := net.Listen("unix", filepath.Join(statePath, socketName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = lis.Close()
	})

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
	t.Helper()

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
	t.Helper()

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
	t.Helper()

	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{
		newWebCommand(nil),
	}
	stdout, stderr, err := captureStdoutStderr(t, func() error {
		return app.RunContext(ctx, append([]string{"spacewave"}, args...))
	})
	if err != nil {
		t.Fatal(err)
	}
	return stdout, stderr
}

func captureStdoutStderr(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

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
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	runErr := fn()
	if err := stdoutW.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := stderrW.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
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
