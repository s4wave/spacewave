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
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	targetPath := newShortSocketPath(t, "daemon.sock")
	listener, err := net.Listen("unix", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

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
	targetPath := newShortSocketPath(t, "daemon.sock")
	listener, err := net.Listen("unix", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

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
				defer conn.Close()
				request := make([]byte, 4)
				if _, err := io.ReadFull(conn, request); err != nil {
					daemonResult <- err
					return
				}
				accepted <- struct{}{}
				<-release
				_, err := conn.Write(request)
				daemonResult <- err
			}()
		}
	}()

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
	for range 2 {
		select {
		case <-accepted:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for concurrent daemon connection")
		}
	}
	close(release)
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
	for range 2 {
		if err := <-daemonResult; err != nil {
			t.Fatal(err)
		}
	}
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}
}

func TestUnixProxyCloseCancelsDial(t *testing.T) {
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
	client, err := net.Dial("unix", proxy.path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for daemon dial")
	}
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if _, err := client.Read(buffer); err == nil {
		t.Fatal("client remained open after cancellation during daemon dial")
	}
}

func TestUnixProxyCloseCancelsConnectionCopies(t *testing.T) {
	targetPath := newShortSocketPath(t, "daemon.sock")
	listener, err := net.Listen("unix", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	proxy, err := startUnixProxy(context.Background(), targetPath)
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.Dial("unix", proxy.path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var daemon net.Conn
	select {
	case daemon = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for daemon connection")
	}
	defer daemon.Close()

	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if _, err := client.Read(buffer); err == nil {
		t.Fatal("client remained open after proxy close")
	}
	if _, err := daemon.Read(buffer); err == nil {
		t.Fatal("daemon connection remained open after proxy close")
	}
}

func TestUnixProxyCloseCancelsAcceptAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	proxy, err := startUnixProxy(context.Background(), filepath.Join(home, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	launchDir := proxy.dir
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyResult(t, proxy.done); err != nil {
		t.Fatal(err)
	}
	if err := proxy.close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
	if _, err := os.Stat(launchDir); !os.IsNotExist(err) {
		t.Fatalf("launch directory still exists: %v", err)
	}
}

func TestUnixProxyReportsDaemonConnectionFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	proxy, err := startUnixProxy(context.Background(), newShortSocketPath(t, "missing.sock"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.Dial("unix", proxy.path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	proxyErr := waitProxyResult(t, proxy.done)
	if proxyErr == nil || !strings.Contains(proxyErr.Error(), "connect private Resource proxy to daemon") {
		t.Fatalf("expected daemon connection error, got %v", proxyErr)
	}
	launchDir := proxy.dir
	if err := proxy.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launchDir); !os.IsNotExist(err) {
		t.Fatalf("launch directory still exists: %v", err)
	}
}

func proxyRoundTrip(t *testing.T, path, request string) string {
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
	t.Helper()
	dir, err := os.MkdirTemp("", "swt-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, name)
}
