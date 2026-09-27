//go:build !js && (darwin || linux)

package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"golang.org/x/sys/unix"
)

// TestReadinessAfterRefusedBind crosses a socket's creation event and refused
// retry before listen, then attaches through Resource Init without another
// socket pathname event. A previous daemon's notification must work too.
func TestReadinessAfterRefusedBind(t *testing.T) {
	for _, previous := range []bool{false, true} {
		name := "first-start"
		if previous {
			name = "previous-notification"
		}
		t.Run(name, func(t *testing.T) {
			// Keep the socket prebound until the connector consumes its sole
			// creation event. Subscribe before binding by using the start seam.
			root := daemonTestRoot(t)
			path := filepath.Join(root, SocketName)
			t.Setenv("SPACEWAVE_SOCKET_PATH", "")
			if previous {
				if err := PublishReady(path); err != nil {
					t.Fatal(err)
				}
			}
			fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
			if err != nil {
				t.Fatal(err)
			}
			file := os.NewFile(uintptr(fd), path)
			t.Cleanup(func() { _ = file.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			refused := make(chan error, 1)
			resume := make(chan struct{})
			attempts := 0
			connector := NewConnector(func(ctx context.Context, path string) (net.Conn, error) {
				// The third dial follows the consumed creation event; retain
				// its refusal until the listener and explicit event are ready.
				conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
				attempts++
				if attempts == 3 {
					refused <- err
					select {
					case <-resume:
					case <-ctx.Done():
					}
				}
				return conn, err
			}, func(context.Context, string) error {
				if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
					return err
				}
				return ErrStarting
			})

			// Connect must pass the real Resource Init exchange, not merely dial.
			connected := make(chan error, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				client, err := connector.Connect(ctx, root, "")
				if client != nil {
					client.Close()
				}
				connected <- err
			}()
			t.Cleanup(func() {
				cancel()
				<-done
			})
			select {
			case err := <-refused:
				if !errors.Is(err, unix.ECONNREFUSED) {
					t.Fatalf("pre-listen retry = %v, want ECONNREFUSED", err)
				}
			case <-ctx.Done():
				t.Fatal("connector did not consume the bind event", ctx.Err())
			}

			// Listen changes the existing socket in place. No chmod, rename,
			// unlink, or other operation touches its pathname after the retry.
			if err := unix.Listen(fd, 8); err != nil {
				t.Fatal(err)
			}
			listener, err := net.FileListener(file)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			mux := srpc.NewMux()
			if err := resource_server.NewResourceServer(nil).Register(mux); err != nil {
				t.Fatal(err)
			}
			served := make(chan struct{})
			go func() {
				defer close(served)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				mc, err := srpc.NewMuxedConn(conn, false, nil)
				if err == nil {
					_ = srpc.NewServer(mux).AcceptMuxedConn(ctx, mc)
				}
			}()
			t.Cleanup(func() {
				cancel()
				_ = listener.Close()
				<-served
			})

			// The event can arrive before the connector starts waiting; its
			// pre-dial subscription must retain it across the refused result.
			if err := PublishReady(path); err != nil {
				t.Fatal(err)
			}
			close(resume)
			if err := <-connected; err != nil {
				t.Fatalf("attach after listener readiness: %v", err)
			}
		})
	}
}
