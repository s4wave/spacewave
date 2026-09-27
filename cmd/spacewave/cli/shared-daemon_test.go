//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fsnotify"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/daemon"
	desktopcontrol "github.com/s4wave/spacewave/core/daemon/desktopcontrol"
)

// TestSharedDaemonStarters exercises real detached processes, state leases and
// Resource streams against both production serve branches.
func TestSharedDaemonStarters(t *testing.T) {
	for _, mode := range []string{"native", "distribution"} {
		t.Run(mode, func(t *testing.T) {
			// Subscribe before launch and constrain every child to a private root.
			statePath := shortSocketDir(t)
			if err := os.Chmod(statePath, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv(sharedDaemonFixtureMode, mode)
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
			_, openErr := desktopcontrol.NewSRPCDesktopControlServiceClient(launcher.RPC()).OpenOrFocusDesktop(ctx, &desktopcontrol.OpenOrFocusDesktopRequest{})
			launcher.Close()
			if openErr == nil {
				t.Fatal("desktop opened without a UI artifact")
			}
			if !strings.Contains(openErr.Error(), "desktop UI artifact unavailable") {
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
