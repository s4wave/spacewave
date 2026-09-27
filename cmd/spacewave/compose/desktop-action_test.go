//go:build !js

package spacewave_compose

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/daemon"
	"github.com/s4wave/spacewave/core/daemon/desktopcontrol"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// desktopActionFixture serves Resource and optional desktop control on one
// protected socket. Its clients have independent Resource stream lifetimes.
type desktopActionFixture struct {
	// listener accepts clients at the isolated daemon root.
	listener net.Listener
	// cancel ends the server and all accepted clients.
	cancel context.CancelFunc
	// finished reports that the accept loop has exited.
	finished chan struct{}
	// events advance a retained Resource stream.
	events chan struct{}
	// control records desktop requests when that capability is installed.
	control *fixtureDesktopControl
}

// fixtureDesktopControl identifies one daemon through each desktop response.
type fixtureDesktopControl struct {
	// mtx guards opens.
	mtx sync.Mutex
	// opens counts acknowledged requests on this daemon.
	opens int
}

// fixtureResourceWatch streams events through the retained Resource client.
type fixtureResourceWatch struct {
	// events supplies the next response.
	events <-chan struct{}
}

// newDesktopActionFixture starts one isolated socket fixture.
func newDesktopActionFixture(t *testing.T, root string, withDesktop bool) *desktopActionFixture {
	t.Helper()
	listener, err := resource_listener.ListenProtectedUnix(filepath.Join(root, daemon.SocketName), true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	fixture := &desktopActionFixture{
		listener: listener,
		cancel:   cancel,
		finished: make(chan struct{}),
		events:   make(chan struct{}, 1),
		control:  &fixtureDesktopControl{},
	}
	rootMux := srpc.NewMux()
	if err := rootMux.Register(&fixtureResourceWatch{events: fixture.events}); err != nil {
		t.Fatal(err)
	}
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(mux); err != nil {
		t.Fatal(err)
	}
	if withDesktop {
		if err := desktopcontrol.SRPCRegisterDesktopControlService(mux, fixture.control); err != nil {
			t.Fatal(err)
		}
	}
	server := srpc.NewServer(mux)
	go func() {
		defer close(fixture.finished)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				mc, err := srpc.NewMuxedConn(conn, false, nil)
				if err == nil {
					_ = server.AcceptMuxedConn(ctx, mc)
				}
			}()
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-fixture.finished
	})
	return fixture
}

// OpenOrFocusDesktop acknowledges a request from the one fixture daemon.
func (c *fixtureDesktopControl) OpenOrFocusDesktop(context.Context, *desktopcontrol.OpenOrFocusDesktopRequest) (*desktopcontrol.OpenOrFocusDesktopResponse, error) {
	c.mtx.Lock()
	c.opens++
	c.mtx.Unlock()
	return &desktopcontrol.OpenOrFocusDesktopResponse{DaemonPid: int64(os.Getpid()), UiManifestRef: "fixture/web"}, nil
}

// WatchDesktopStatus is unused by the launcher's one-shot operation.
func (c *fixtureDesktopControl) WatchDesktopStatus(*desktopcontrol.WatchDesktopStatusRequest, desktopcontrol.SRPCDesktopControlService_WatchDesktopStatusStream) error {
	return nil
}

// openCount returns acknowledged requests on this daemon.
func (c *fixtureDesktopControl) openCount() int {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.opens
}

// GetServiceID identifies the fixture's root Resource service.
func (w *fixtureResourceWatch) GetServiceID() string { return "test.DesktopActionWatch" }

// GetMethodIDs lists the fixture's watch method.
func (w *fixtureResourceWatch) GetMethodIDs() []string { return []string{"Watch"} }

// InvokeMethod advances one Resource stream without depending on launcher lifetime.
func (w *fixtureResourceWatch) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	if serviceID != w.GetServiceID() || methodID != "Watch" {
		return false, nil
	}
	if err := stream.MsgRecv(&emptypb.Empty{}); err != nil {
		return true, err
	}
	for {
		select {
		case <-w.events:
			if err := stream.MsgSend(&emptypb.Empty{}); err != nil {
				return true, err
			}
		case <-stream.Context().Done():
			return true, stream.Context().Err()
		}
	}
}

// TestDesktopActionStartsOrAttachesWithoutClosingOtherResources checks both
// launch orders through the composed action and the shared Connector.
func TestDesktopActionStartsOrAttachesWithoutClosingOtherResources(t *testing.T) {
	if Compose().NativeAction == nil {
		t.Fatal("Spacewave native composition omits the desktop action")
	}
	for _, order := range []string{"desktop-first", "client-first"} {
		t.Run(order, func(t *testing.T) {
			root := desktopActionStatePath(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			var fixture *desktopActionFixture
			starts := 0
			connector := daemon.NewConnector(nil, func(_ context.Context, statePath string) error {
				if statePath != root {
					t.Fatalf("starter state root = %q, want %q", statePath, root)
				}
				starts++
				fixture = newDesktopActionFixture(t, root, true)
				return daemon.PublishReady(filepath.Join(root, daemon.SocketName))
			})

			if order == "desktop-first" {
				if err := openDesktopWithConnector(ctx, connector); err != nil {
					t.Fatal(err)
				}
			}
			retained, err := connector.Connect(ctx, "", "")
			if err != nil {
				t.Fatal(err)
			}
			defer retained.Close()
			if order == "client-first" {
				if err := openDesktopWithConnector(ctx, connector); err != nil {
					t.Fatal(err)
				}
			}
			if starts != 1 {
				t.Fatalf("daemon starts = %d, want 1", starts)
			}
			if opens := fixture.control.openCount(); opens != 1 {
				t.Fatalf("desktop acknowledgements = %d, want 1", opens)
			}

			rootRPC, err := retained.Root().GetResourceRef().GetClient()
			if err != nil {
				t.Fatal(err)
			}
			stream, err := rootRPC.NewStream(ctx, "test.DesktopActionWatch", "Watch", &emptypb.Empty{})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if err := stream.CloseSend(); err != nil {
				t.Fatal(err)
			}
			fixture.events <- struct{}{}
			if err := stream.MsgRecv(&emptypb.Empty{}); err != nil {
				t.Fatalf("retained Resource stream after desktop launcher exit: %v", err)
			}
		})
	}
}

// TestDesktopActionExplicitSocketNeverStarts checks connect-only selection even
// when the selected state root has no daemon of its own.
func TestDesktopActionExplicitSocketNeverStarts(t *testing.T) {
	root := desktopActionStatePath(t)
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := newDesktopActionFixture(t, other, true)
	t.Setenv("SPACEWAVE_SOCKET_PATH", filepath.Join(other, daemon.SocketName))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := Compose().NativeAction(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if opens := fixture.control.openCount(); opens != 1 {
		t.Fatalf("explicit socket desktop acknowledgements = %d, want 1", opens)
	}
	if _, err := os.Stat(filepath.Join(root, daemon.SocketName)); !os.IsNotExist(err) {
		t.Fatalf("explicit socket created a daemon in the state root: %v", err)
	}
}

// TestDesktopActionMissingCapabilityRequiresUpgrade checks that a Resource
// daemon without desktop control is never replaced by the launcher.
func TestDesktopActionMissingCapabilityRequiresUpgrade(t *testing.T) {
	root := desktopActionStatePath(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	starts := 0
	connector := daemon.NewConnector(nil, func(_ context.Context, statePath string) error {
		if statePath != root {
			t.Fatalf("starter state root = %q, want %q", statePath, root)
		}
		starts++
		newDesktopActionFixture(t, root, false)
		return daemon.PublishReady(filepath.Join(root, daemon.SocketName))
	})
	err := openDesktopWithConnector(ctx, connector)
	if err == nil || !strings.Contains(err.Error(), "upgrade and restart") || starts != 1 {
		t.Fatalf("missing desktop capability: error=%v starts=%d", err, starts)
	}
	err = openDesktopWithConnector(ctx, connector)
	if err == nil || !strings.Contains(err.Error(), "upgrade and restart") || starts != 1 {
		t.Fatalf("repeat launch took over daemon: error=%v starts=%d", err, starts)
	}
}

// desktopActionStatePath constrains every fixture to a short worktree root.
func desktopActionStatePath(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "..", ".tmp")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "desktop-entry-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPACEWAVE_STATE_PATH", root)
	t.Setenv("SPACEWAVE_SOCKET_PATH", "")
	return root
}

// _ is a type assertion.
var (
	_ desktopcontrol.SRPCDesktopControlServiceServer = (*fixtureDesktopControl)(nil)
	_ srpc.Invoker                                   = (*fixtureResourceWatch)(nil)
)
