//go:build !js && !windows

package bldr_tui_host

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnixProxySecuresForwardsSequentialConnectionsAndCleansLaunch(t *testing.T) {
	// Prepare an isolated cache and daemon socket for the proxy test.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	targetPath := newShortSocketPath(t, "daemon.sock")
	listener, err := net.Listen("unix", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Start a daemon that reads each request and returns a response.
	daemonResult := make(chan error, 1)
	received := make(chan []byte, 2)
	go func() {
		for range 2 {
			conn, err := listener.Accept()
			if err != nil {
				daemonResult <- err
				return
			}
			request := make([]byte, 4)
			if _, err := io.ReadFull(conn, request); err != nil {
				_ = conn.Close()
				daemonResult <- err
				return
			}
			received <- request
			_, writeErr := conn.Write([]byte("pong"))
			closeErr := conn.Close()
			if writeErr != nil {
				daemonResult <- writeErr
				return
			}
			if closeErr != nil {
				daemonResult <- closeErr
				return
			}
		}
		daemonResult <- nil
	}()

	// Launch the private proxy and check its socket permissions.
	ctx := t.Context()
	proxy, err := startUnixProxy(ctx, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	launchDir := proxy.dir
	if stat, err := os.Stat(launchDir); err != nil {
		t.Fatal(err)
	} else if mode := stat.Mode().Perm(); mode != 0o700 {
		t.Fatalf("launch directory mode = %o", mode)
	}
	if stat, err := os.Stat(proxy.path); err != nil {
		t.Fatal(err)
	} else if mode := stat.Mode().Perm(); mode != 0o600 {
		t.Fatalf("proxy socket mode = %o", mode)
	}

	// Exchange sequential requests through the proxy and verify forwarding.
	for _, request := range []string{"ping", "ring"} {
		if response := proxyRoundTrip(t, proxy.path, request); response != "pong" {
			t.Fatalf("proxy response = %q", response)
		}
		select {
		case receivedRequest := <-received:
			if !bytes.Equal(receivedRequest, []byte(request)) {
				t.Fatalf("daemon request = %q", string(receivedRequest))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for daemon request")
		}
	}
	if _, err := os.Stat(proxy.path); err != nil {
		t.Fatalf("launch socket was unlinked while proxy remained active: %v", err)
	}
	select {
	case err := <-proxy.done:
		t.Fatalf("proxy stopped after a connection ended: %v", err)
	default:
	}
	if err := <-daemonResult; err != nil {
		t.Fatal(err)
	}
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launchDir); !os.IsNotExist(err) {
		t.Fatalf("launch directory still exists: %v", err)
	}
}

func TestUnixProxyForwardsConcurrentConnections(t *testing.T) {
	// Create the daemon listener used for concurrent connections.
	targetPath := newShortSocketPath(t, "daemon.sock")
	listener, err := net.Listen("unix", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Coordinate the daemon handlers so both connections remain open together.
	accepted := make(chan struct{}, 2)
	release := make(chan struct{})
	daemonResult := make(chan error, 2)
	go func() {
		for range 2 {
			conn, err := listener.Accept()
			if err != nil {
				daemonResult <- err
				return
			}
			go func() {
				// Close each daemon connection when its request handler exits.
				defer conn.Close()

				// Read the complete request from the daemon connection.
				request := make([]byte, 4)
				if _, err := io.ReadFull(conn, request); err != nil {
					daemonResult <- err
					return
				}

				// Signal that the request arrived, then wait before replying.
				accepted <- struct{}{}
				<-release
				_, err := conn.Write(request)
				daemonResult <- err
			}()
		}
	}()

	// Start the proxy and create two clients for concurrent forwarding.
	proxy, err := startUnixProxy(context.Background(), targetPath)
	if err != nil {
		t.Fatal(err)
	}
	clients := make([]net.Conn, 0, 2)
	for _, request := range []string{"one1", "two2"} {
		client, err := net.Dial("unix", proxy.path)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
		if _, err := client.Write([]byte(request)); err != nil {
			t.Fatal(err)
		}
	}

	// Wait until the daemon has accepted both proxied connections.
	for range 2 {
		select {
		case <-accepted:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for concurrent daemon connection")
		}
	}

	// Release both daemon handlers after they have accepted their requests.
	close(release)

	// Read and verify each response before closing its client.
	for idx, client := range clients {
		response := make([]byte, 4)
		if _, err := io.ReadFull(client, response); err != nil {
			t.Fatal(err)
		}
		expected := []string{"one1", "two2"}[idx]
		if string(response) != expected {
			t.Fatalf("response %d = %q, expected %q", idx, string(response), expected)
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Collect the results from both daemon handlers.
	for range 2 {
		if err := <-daemonResult; err != nil {
			t.Fatal(err)
		}
	}

	// Close the proxy and wait for its serving goroutine to finish.
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}
}

func TestUnixProxyCloseCancelsDial(t *testing.T) {
	// Configure a dialer that blocks until the proxy context is canceled.
	dialStarted := make(chan struct{})
	proxy, err := startUnixProxyWithDial(
		context.Background(),
		newShortSocketPath(t, "daemon.sock"),
		func(ctx context.Context, _, _ string) (net.Conn, error) {
			close(dialStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Connect a client to trigger the blocked daemon dial.
	client, err := net.Dial("unix", proxy.path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Wait until the proxy begins dialing the daemon.
	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for daemon dial")
	}

	// Close the proxy while its daemon dial is blocked.
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}

	// Verify that cancellation closes the waiting client connection.
	buffer := make([]byte, 1)
	if _, err := client.Read(buffer); err == nil {
		t.Fatal("client remained open after cancellation during daemon dial")
	}
}

func TestUnixProxyCloseCancelsConnectionCopies(t *testing.T) {
	// Create a daemon listener for the proxied connection-copy test.
	targetPath := newShortSocketPath(t, "daemon.sock")
	listener, err := net.Listen("unix", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Accept and publish the daemon connection for the test.
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	// Start the proxy and connect its client to the daemon.
	proxy, err := startUnixProxy(context.Background(), targetPath)
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.Dial("unix", proxy.path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Wait for the daemon side of the proxied connection.
	var daemon net.Conn
	select {
	case daemon = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for daemon connection")
	}
	defer daemon.Close()

	// Close the proxy and wait for both connection copies to stop.
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}

	// Verify that proxy shutdown closes both client and daemon sockets.
	buffer := make([]byte, 1)
	if _, err := client.Read(buffer); err == nil {
		t.Fatal("client remained open after proxy close")
	}
	if _, err := daemon.Read(buffer); err == nil {
		t.Fatal("daemon connection remained open after proxy close")
	}
}

func TestUnixProxyCloseCancelsAcceptAndIsIdempotent(t *testing.T) {
	// Start an idle proxy with an isolated runtime directory.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	proxy, err := startUnixProxy(context.Background(), filepath.Join(home, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	launchDir := proxy.dir

	// Close the idle proxy to cancel its accept loop.
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}

	// Wait for the canceled accept loop to finish.
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}

	// Close the proxy a second time to verify idempotency.
	if err := proxy.close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}

	// Verify that shutdown removed the private runtime directory.
	if _, err := os.Stat(launchDir); !os.IsNotExist(err) {
		t.Fatalf("launch directory still exists: %v", err)
	}
}

func TestUnixProxyReportsDaemonConnectionFailure(t *testing.T) {
	// Start a proxy whose target daemon socket does not exist.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	proxy, err := startUnixProxy(context.Background(), newShortSocketPath(t, "missing.sock"))
	if err != nil {
		t.Fatal(err)
	}

	// Connect a client to trigger the failed daemon dial.
	client, err := net.Dial("unix", proxy.path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Wait for and inspect the daemon connection failure.
	proxyErr := waitProxyResult(t, proxy.done)
	if proxyErr == nil || !strings.Contains(proxyErr.Error(), "connect private Resource proxy to daemon") {
		t.Fatalf("expected daemon connection error, got %v", proxyErr)
	}

	// Close the failed proxy and retain its runtime directory for cleanup checks.
	launchDir := proxy.dir
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launchDir); !os.IsNotExist(err) {
		t.Fatalf("launch directory still exists: %v", err)
	}
}

func proxyRoundTrip(t *testing.T, path, request string) string {
	// Send one request through the Unix proxy and return its complete response.
	t.Helper()
	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(client, response); err != nil {
		t.Fatal(err)
	}
	return string(response)
}

func waitProxyResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for private Resource proxy")
		return nil
	}
}

func newShortSocketPath(t *testing.T, name string) string {
	// Create a temporary directory for the short Unix socket path.
	t.Helper()
	dir, err := os.MkdirTemp("", "swt-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, name)
}
