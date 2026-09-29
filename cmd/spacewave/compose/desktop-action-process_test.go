//go:build !js

package spacewave_compose

import (
	"context"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/daemon"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// copiedDaemonFixtureEnv selects the detached test executable's serve fixture.
const copiedDaemonFixtureEnv = "SPACEWAVE_TEST_COPIED_DAEMON"

// copiedDaemonFixtureFailEnv rejects a child before its readiness acknowledgement.
const copiedDaemonFixtureFailEnv = "SPACEWAVE_TEST_COPIED_DAEMON_FAIL"

// TestMain lets StartCopiedProcess execute this built binary from a fake app.
func TestMain(m *testing.M) {
	if os.Getenv(copiedDaemonFixtureEnv) == "1" && len(os.Args) > 5 && os.Args[1] == "--state-path" {
		if err := runCopiedDaemonFixture(os.Args[2], os.Args[5]); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runCopiedDaemonFixture serves Resource and DesktopControl after the same
// private startup acknowledgement used by production detached daemons.
func runCopiedDaemonFixture(statePath, pipeID string) (retErr error) {
	// Give the first child exclusive fixture ownership of this state root.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	notifier, err := daemon.NewStartupNotifier(ctx, statePath, pipeID)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			notifier.Error(retErr)
		}
		notifier.Close()
	}()
	ownerPath := filepath.Join(statePath, "fixture-owner")
	owner, err := os.OpenFile(ownerPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return daemon.ErrStarting
	}
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(ownerPath); err != nil && retErr == nil {
			retErr = err
		}
		if err := os.WriteFile(filepath.Join(statePath, "fixture-stopped"), nil, 0o600); err != nil && retErr == nil {
			retErr = err
		}
	}()
	if _, err := owner.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = owner.Close()
		return err
	}
	if err := owner.Close(); err != nil {
		return err
	}
	if os.Getenv(copiedDaemonFixtureFailEnv) == "1" {
		return errors.New("injected fixture startup failure")
	}

	// Bind both services, but wait for custody transfer before accepting clients.
	listener, err := resource_listener.ListenProtectedUnix(filepath.Join(statePath, daemon.SocketName), true)
	if err != nil {
		return err
	}
	defer listener.Close()
	rootMux := srpc.NewMux()
	if err := rootMux.Register(&fixtureResourceWatch{events: make(chan struct{})}); err != nil {
		return err
	}
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(mux); err != nil {
		return err
	}
	if err := desktop_control.SRPCRegisterDesktopControlService(mux, &fixtureDesktopControl{}); err != nil {
		return err
	}
	if err := notifier.Ready(ctx); err != nil {
		return err
	}
	if err := daemon.PublishReady(filepath.Join(statePath, daemon.SocketName)); err != nil {
		return err
	}

	// Serve attached clients until this fixture's parent requests shutdown.
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	server := srpc.NewServer(mux)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				return err
			}
			return nil
		}
		go func(conn net.Conn) {
			defer conn.Close()
			mc, err := srpc.NewMuxedConn(conn, false, nil)
			if err == nil {
				_ = server.AcceptMuxedConn(ctx, mc)
			}
		}(conn)
	}
}

// TestDesktopStarterUsesCopiedBundle proves copied bootstrap, concurrent
// attachment, and later attachment without the installed bundle source.
func TestDesktopStarterUsesCopiedBundle(t *testing.T) {
	// Copy this built test executable into an isolated fake app bundle.
	statePath := desktopActionStatePath(t)
	t.Setenv(copiedDaemonFixtureEnv, "1")
	t.Setenv(daemon.StartupTimeoutEnvVar, "15s")
	bundle := filepath.Join(statePath, "Spacewave.app", "Contents", "MacOS")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bundle, "spacewave")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(source, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}

	// Observe this child only, including its process-exit notification.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	if err := watcher.Add(statePath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(statePath, "fixture-owner"))
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil {
			t.Error(err)
			return
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			t.Error(err)
			return
		}
		if err := process.Signal(os.Interrupt); err != nil {
			t.Error(err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for {
			if _, err := os.Stat(filepath.Join(statePath, "fixture-stopped")); err == nil {
				return
			}
			select {
			case <-watcher.Events:
			case err := <-watcher.Errors:
				t.Error(err)
				return
			case <-ctx.Done():
				t.Error(ctx.Err())
				return
			}
		}
	})

	// Concurrent desktop requests converge on one detached daemon.
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	connector := daemon.NewConnector(nil, func(ctx context.Context, root string) error {
		return daemon.StartCopiedProcess(ctx, root, source)
	})
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- openDesktopWithConnector(ctx, connector) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	client, err := connector.Connect(ctx, statePath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	first, err := desktop_control.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(statePath, "daemon-bin")
	if first.GetDaemonPid() <= 0 || !strings.HasPrefix(first.GetDaemonExecutable(), copied) || !strings.HasSuffix(first.GetDaemonExecutable(), filepath.Join(".app", "Contents", "MacOS", "spacewave")) {
		t.Fatalf("daemon identity: pid=%d executable=%q", first.GetDaemonPid(), first.GetDaemonExecutable())
	}
	if first.GetUiManifestRef() != "fixture/web" {
		t.Fatalf("desktop artifact = %q", first.GetUiManifestRef())
	}

	// Removing the app source makes any second copy impossible; attach still works.
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := openDesktopWithConnector(ctx, connector); err != nil {
		t.Fatal(err)
	}
	second, err := desktop_control.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if second.GetDaemonPid() != first.GetDaemonPid() || second.GetDaemonExecutable() != first.GetDaemonExecutable() {
		t.Fatalf("second launch changed daemon: first=%v second=%v", first, second)
	}
}

// TestDesktopStarterFailureLeavesVerifiedCopy checks that failed startup joins
// its child and leaves no published readiness or damaged executable behind.
func TestDesktopStarterFailureLeavesVerifiedCopy(t *testing.T) {
	statePath := desktopActionStatePath(t)
	t.Setenv(copiedDaemonFixtureEnv, "1")
	t.Setenv(copiedDaemonFixtureFailEnv, "1")
	t.Setenv(daemon.StartupTimeoutEnvVar, "10s")
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := daemon.StartCopiedProcess(ctx, statePath, source); err == nil || !strings.Contains(err.Error(), "injected fixture startup failure") {
		t.Fatalf("failed child startup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(statePath, "fixture-owner")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed child retained ownership: %v", err)
	}
	if _, err := os.Stat(filepath.Join(statePath, daemon.SocketName) + ".ready"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed child published readiness: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(statePath, "daemon-bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Type().IsRegular() {
		t.Fatalf("failed startup left incomplete executable copies: %v", entries)
	}
}
