//go:build !js

package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// daemonTestRoot creates isolated state with a short Unix socket path.
func daemonTestRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("SPACEWAVE_TEST_STATE_ROOT")
	if root == "" {
		root = ".tmp"
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path, err := os.MkdirTemp(root, "sw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })
	return path
}

// TestExplicitMissingSocketNeverStarts proves connect-only targets cannot launch
// a process or create a writable state root.
func TestExplicitMissingSocketNeverStarts(t *testing.T) {
	root := daemonTestRoot(t)
	state := filepath.Join(root, "uncreated")
	connector := NewConnector(nil, func(context.Context, string) error {
		t.Fatal("explicit socket started a daemon")
		return nil
	})
	if _, err := connector.Connect(t.Context(), state, filepath.Join(root, "missing.sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing socket result: %v", err)
	}
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("connect-only target created state: %v", err)
	}
}

// TestDialErrorsNeverAuthorizeStartup preserves socket paths on permission and
// other transport failures, even when there is no successful liveness probe.
func TestDialErrorsNeverAuthorizeStartup(t *testing.T) {
	for _, failure := range []error{os.ErrPermission, syscall.EIO, context.Canceled} {
		root := daemonTestRoot(t)
		path := filepath.Join(root, SocketName)
		if err := os.WriteFile(path, []byte("untouched"), 0o600); err != nil {
			t.Fatal(err)
		}
		connector := NewConnector(func(context.Context, string) (net.Conn, error) {
			return nil, failure
		}, func(context.Context, string) error {
			t.Fatal("transport error authorized startup")
			return nil
		})
		if _, err := connector.Connect(t.Context(), root, ""); !errors.Is(err, failure) {
			t.Fatalf("failure %v became %v", failure, err)
		}
		if contents, err := os.ReadFile(path); err != nil || string(contents) != "untouched" {
			t.Fatalf("existing socket path changed: %q, %v", contents, err)
		}
	}
}

// TestProtocolFailureNeverStarts leaves a reachable incompatible server intact.
func TestProtocolFailureNeverStarts(t *testing.T) {
	// Serve SRPC without ResourceService to produce a real protocol error.
	root := daemonTestRoot(t)
	path := filepath.Join(root, SocketName)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	served := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			served <- err
			return
		}
		defer conn.Close()
		mc, err := srpc.NewMuxedConn(conn, false, nil)
		if err == nil {
			err = srpc.NewServer(srpc.NewMux()).AcceptMuxedConn(t.Context(), mc)
		}
		served <- err
	}()

	// Initialization must fail through the existing socket, without a child.
	connector := NewConnector(nil, func(context.Context, string) error {
		t.Fatal("protocol failure started another daemon")
		return nil
	})
	if client, err := connector.Connect(t.Context(), root, ""); err == nil {
		client.Close()
		t.Fatal("missing Resource service unexpectedly initialized")
	}
	<-served
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("protocol failure removed live socket: %v", err)
	}
}
