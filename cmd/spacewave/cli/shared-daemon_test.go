//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fsnotify"
	"github.com/pkg/errors"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/daemon"
)

// TestAppBundleReplacementRetainsSharedDaemon starts from a separate app copy,
// swaps that copy, and keeps the copied daemon's Resource watch alive.
func TestAppBundleReplacementRetainsSharedDaemon(t *testing.T) {
	// Keep every fixture path under this checkout's disposable state root.
	tmpRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	statePath, err := os.MkdirTemp(tmpRoot, "app-update-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(statePath) })
	t.Setenv(sharedDaemonFixtureMode, "native")
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)
	t.Setenv("SPACEWAVE_SOCKET_PATH", "")
	t.Setenv(daemonIdleTimeoutEnvVar, "30s")
	t.Setenv(daemon.StartupTimeoutEnvVar, "15s")
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Add(statePath); err != nil {
		t.Fatal(err)
	}

	// Copy the executable into a fake installed bundle before desktop startup.
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	installedApp := filepath.Join(statePath, "Spacewave.app")
	appBinary := filepath.Join(installedApp, "Contents", "MacOS", "spacewave")
	if err := os.MkdirAll(filepath.Dir(appBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(appBinary, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
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

	// Start a digest-named daemon copy and retain its live Resource stream.
	connector := daemon.NewConnector(nil, func(ctx context.Context, root string) error {
		return daemon.StartCopiedProcess(ctx, root, appBinary)
	})
	client, err := connector.Connect(ctx, statePath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer stopSharedDaemonFixture(t, statePath, watcher)
	defer client.Close()
	atom := sharedDaemonAtom(t, client)
	stream, err := atom.WatchState(ctx, &resource_state.WatchStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	pidBefore, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
	if err != nil {
		t.Fatal(err)
	}
	executableBefore, err := os.ReadFile(filepath.Join(statePath, "runtime-executable"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(executableBefore), filepath.Join(statePath, "daemon-bin")+string(filepath.Separator)) {
		t.Fatalf("daemon executable remained in app bundle: %q", executableBefore)
	}
	daemonFileBefore, err := os.Stat(string(executableBefore))
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(statePath, socketName)
	socketBefore, err := os.Stat(socketPath)
	if err != nil {
		t.Fatal(err)
	}

	// Swap only the app copy while the daemon and its watch remain attached.
	stagedApp := filepath.Join(statePath, "Staged.app")
	if err := os.MkdirAll(stagedApp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagedApp, "new-version"), []byte("updated app"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(installedApp, filepath.Join(statePath, "Old.app")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stagedApp, installedApp); err != nil {
		t.Fatal(err)
	}
	if _, err := atom.SetState(ctx, &resource_state.SetStateRequest{StateJson: `"after-app-update"`}); err != nil {
		t.Fatal(err)
	}
	value, err := stream.Recv()
	if err != nil || value.GetStateJson() != `"after-app-update"` {
		t.Fatalf("retained watch after app swap: value=%v error=%v", value, err)
	}

	// The daemon PID, executable inode, and protected socket retain identity.
	pidAfter, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
	if err != nil || string(pidAfter) != string(pidBefore) {
		t.Fatalf("daemon PID changed from %q to %q: %v", pidBefore, pidAfter, err)
	}
	executableAfter, err := os.ReadFile(filepath.Join(statePath, "runtime-executable"))
	if err != nil || string(executableAfter) != string(executableBefore) {
		t.Fatalf("daemon executable changed from %q to %q: %v", executableBefore, executableAfter, err)
	}
	daemonFileAfter, err := os.Stat(string(executableAfter))
	if err != nil || !os.SameFile(daemonFileBefore, daemonFileAfter) {
		t.Fatalf("daemon executable inode changed: %v", err)
	}
	socketAfter, err := os.Stat(socketPath)
	if err != nil || !os.SameFile(socketBefore, socketAfter) {
		t.Fatalf("daemon socket changed: %v", err)
	}
}

// TestSharedDaemonStarters exercises real detached processes, state leases and
// Resource streams against both production serve branches.
func TestSharedDaemonStarters(t *testing.T) {
	for _, mode := range []string{"native", "distribution"} {
		t.Run(mode, func(t *testing.T) {
			// Constrain every child to a private state root.
			statePath := shortSocketDir(t)
			if err := os.Chmod(statePath, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv(sharedDaemonFixtureMode, mode)
			t.Setenv("SPACEWAVE_STATE_PATH", statePath)
			t.Setenv("SPACEWAVE_SOCKET_PATH", "")
			t.Setenv(daemonIdleTimeoutEnvVar, "30s")
			t.Setenv(daemon.StartupTimeoutEnvVar, "15s")

			// Subscribe to the state root before launch.
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
			defer cancel()
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				t.Fatal(err)
			}
			defer watcher.Close()
			if err := watcher.Add(statePath); err != nil {
				t.Fatal(err)
			}

			// Force both callers through the same observed-absence boundary.
			start := make(chan struct{})
			arrived := make(chan struct{}, 2)
			connector := daemon.NewConnector(nil, func(ctx context.Context, root string) error {
				arrived <- struct{}{}
				select {
				case <-start:
				case <-ctx.Done():
					return ctx.Err()
				}
				return daemon.StartProcess(ctx, root)
			})

			// Launch both starters and release them together once both arrive.
			type result struct {
				// client retains a successfully initialized starter connection.
				client *daemon.Client
				// err reports startup or Resource initialization failure.
				err error
			}
			results := make(chan result, 2)
			launch := func() {
				client, err := connector.Connect(ctx, statePath, "")
				results <- result{client: client, err: err}
			}
			go launch()
			go launch()
			for range 2 {
				select {
				case <-arrived:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			close(start)

			// Retain both clients and stop the daemon when the test ends.
			first, second := <-results, <-results
			if first.client != nil {
				defer first.client.Close()
			}
			if second.client != nil {
				defer second.client.Close()
			}
			if first.client != nil || second.client != nil {
				defer stopSharedDaemonFixture(t, statePath, watcher)
			}
			if first.err != nil || second.err != nil {
				t.Fatalf("starters: first=%v second=%v", first.err, second.err)
			}

			// Both production serve branches publish readiness before accepting
			// clients, independently of the earlier socket pathname event.
			readiness, err := os.ReadFile(filepath.Join(statePath, socketName) + ".ready")
			if err != nil || string(readiness) != "ready\n" {
				t.Fatalf("daemon readiness notification: %q, %v", readiness, err)
			}

			// Both clients read the one bus's durable identity through Resource Init.
			identity, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
			if err != nil {
				t.Fatal(err)
			}
			if lease, err := acquireStatePathLease(statePath); !errors.Is(err, daemon.ErrStarting) {
				if lease != nil {
					_ = lease.release()
				}
				t.Fatalf("daemon did not retain its unique state lease: %v", err)
			}
			firstAtom := sharedDaemonAtom(t, first.client)
			secondAtom := sharedDaemonAtom(t, second.client)
			for _, atom := range []resource_state.SRPCStateAtomResourceServiceClient{firstAtom, secondAtom} {
				value, err := atom.GetState(ctx, &resource_state.GetStateRequest{})
				if err != nil || value.GetStateJson() != string(identity) {
					t.Fatalf("daemon identity: value=%v error=%v, want %s", value, err, identity)
				}
			}

			// Keep a live watch while the other attached client closes.
			stream, err := secondAtom.WatchState(ctx, &resource_state.WatchStateRequest{})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if _, err := stream.Recv(); err != nil {
				t.Fatal(err)
			}
			first.client.Close()
			if _, err := secondAtom.SetState(ctx, &resource_state.SetStateRequest{StateJson: `"after-close"`}); err != nil {
				t.Fatal(err)
			}
			value, err := stream.Recv()
			if err != nil || value.GetStateJson() != `"after-close"` {
				t.Fatalf("retained watch after client close: value=%v error=%v", value, err)
			}

			// A missing UI artifact fails only the desktop request. The launcher
			// disconnects while the other client's Resource watch remains live.
			launcher, err := daemon.Connect(ctx, statePath, filepath.Join(statePath, socketName))
			if err != nil {
				t.Fatal(err)
			}
			_, openErr := desktop_control.NewSRPCDesktopControlServiceClient(launcher.RPC()).OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{})
			launcher.Close()
			if openErr == nil {
				t.Fatal("desktop opened without a UI artifact")
			}
			if openErr.Error() != ErrDesktopUIUnavailable.Error() {
				t.Fatalf("missing desktop artifact: %v", openErr)
			}

			// Advance the retained Resource watch after the desktop request fails.
			if _, err := secondAtom.SetState(ctx, &resource_state.SetStateRequest{StateJson: `"after-desktop-failure"`}); err != nil {
				t.Fatal(err)
			}
			value, err = stream.Recv()
			if err != nil {
				t.Fatal(err)
			}
			if value.GetStateJson() != `"after-desktop-failure"` {
				t.Fatalf("retained watch after desktop failure: value=%v error=%v", value, err)
			}
			t.Logf("%s: daemon %s kept its Resource watch after a missing-artifact desktop request", mode, identity)
		})
	}
}

// sharedDaemonAtom accesses the real atom served as the fixture's root resource.
func sharedDaemonAtom(t *testing.T, client *daemon.Client) resource_state.SRPCStateAtomResourceServiceClient {
	// Acquire the root's routed Resource client for atom operations.
	t.Helper()
	rpc, err := client.Root().GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	return resource_state.NewSRPCStateAtomResourceServiceClient(rpc)
}

// stopSharedDaemonFixture requests authorized fixture shutdown and waits for
// the child's post-bus-release event, never inspecting unrelated processes.
func stopSharedDaemonFixture(t *testing.T, statePath string, watcher *fsnotify.Watcher) {
	// Request shutdown only through this fixture's isolated socket.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(statePath, socketName))
	if err != nil {
		t.Error(err)
		return
	}
	err = requestDaemonShutdown(ctx, conn)
	_ = conn.Close()
	if err != nil {
		t.Error(err)
		return
	}

	// Observe the fixture's post-release event before removing its test root.
	for {
		if _, err := os.Stat(filepath.Join(statePath, "stopped")); err == nil {
			return
		}
		select {
		case <-watcher.Events:
		case err := <-watcher.Errors:
			t.Error(err)
			return
		case <-ctx.Done():
			t.Error("fixture did not release its bus and lease", ctx.Err())
			return
		}
	}
}

// TestSharedDaemonSurvivesStarterCancellation crosses the ready transfer before
// canceling the original launcher, while another client's watch remains open.
func TestSharedDaemonSurvivesStarterCancellation(t *testing.T) {
	for _, mode := range []string{"native", "distribution"} {
		t.Run(mode, func(t *testing.T) {
			// Bound the fixture independently of the launcher's canceled context.
			statePath := shortSocketDir(t)
			t.Setenv(sharedDaemonFixtureMode, mode)
			t.Setenv("SPACEWAVE_STATE_PATH", statePath)
			t.Setenv("SPACEWAVE_SOCKET_PATH", "")
			t.Setenv(daemon.StartupTimeoutEnvVar, "15s")
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			starterCtx, cancelStarter := context.WithCancel(ctx)
			defer cancelStarter()
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				t.Fatal(err)
			}
			defer watcher.Close()
			if err := watcher.Add(statePath); err != nil {
				t.Fatal(err)
			}

			// Attach a second client after custody transfer, then fail the starter
			// before its post-start dial and Resource Init can succeed.
			var retained *daemon.Client
			var stream resource_state.SRPCStateAtomResourceService_WatchStateClient
			connector := daemon.NewConnector(nil, func(startCtx context.Context, root string) error {
				if err := daemon.StartProcess(startCtx, root); err != nil {
					return err
				}
				defer stopSharedDaemonOnFailedAttach(t, &retained, root, watcher)
				var err error
				retained, err = daemon.Connect(ctx, root, filepath.Join(root, socketName))
				if err != nil {
					return err
				}
				stream, err = sharedDaemonAtom(t, retained).WatchState(ctx, &resource_state.WatchStateRequest{})
				if err != nil {
					return err
				}
				if _, err := stream.Recv(); err != nil {
					return err
				}
				cancelStarter()
				return nil
			})
			client, err := connector.Connect(starterCtx, statePath, "")
			if client != nil {
				client.Close()
				t.Fatal("canceled starter returned a client")
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("starter failure = %v, want cancellation", err)
			}
			if retained == nil {
				t.Fatal("second client did not attach before starter failed")
			}
			defer retained.Close()
			defer stream.Close()
			defer stopSharedDaemonFixture(t, statePath, watcher)

			// Advance the same stream after starter failure, proving the process
			// and its independently adopted resources are still usable.
			atom := sharedDaemonAtom(t, retained)
			if _, err := atom.SetState(ctx, &resource_state.SetStateRequest{StateJson: `"after-starter-failure"`}); err != nil {
				t.Fatal(err)
			}
			value, err := stream.Recv()
			if err != nil || value.GetStateJson() != `"after-starter-failure"` {
				t.Fatalf("retained stream after starter failure: value=%v error=%v", value, err)
			}
			t.Logf("%s: retained Resource stream advanced after original starter cancellation", mode)
		})
	}
}

// stopSharedDaemonOnFailedAttach cleans up a fixture whose post-ready attachment
// failed before the test could register its normal shutdown.
func stopSharedDaemonOnFailedAttach(t *testing.T, client **daemon.Client, statePath string, watcher *fsnotify.Watcher) {
	t.Helper()
	if *client == nil {
		stopSharedDaemonFixture(t, statePath, watcher)
	}
}
