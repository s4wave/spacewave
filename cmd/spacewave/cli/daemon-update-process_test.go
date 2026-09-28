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
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/daemon"
)

// TestAcceptedDaemonUpdateRelaunchesAfterFinalClient crosses the real Resource
// stream, state lease, and startup acknowledgement with controlled CLI copies.
func TestAcceptedDaemonUpdateRelaunchesAfterFinalClient(t *testing.T) {
	for _, mode := range []string{"native", "distribution"} {
		t.Run(mode, func(t *testing.T) {
			testAcceptedDaemonUpdateRelaunchesAfterFinalClient(t, mode)
		})
	}
}

// testAcceptedDaemonUpdateRelaunchesAfterFinalClient exercises each core shape.
func testAcceptedDaemonUpdateRelaunchesAfterFinalClient(t *testing.T, mode string) {
	// Keep both executable copies and the writable root inside this checkout.
	statePath := shortSocketDir(t)
	t.Setenv(sharedDaemonFixtureMode, mode)
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)
	t.Setenv("SPACEWAVE_SOCKET_PATH", "")
	t.Setenv(daemonIdleTimeoutEnvVar, "30s")
	t.Setenv(daemon.StartupTimeoutEnvVar, "15s")
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Add(statePath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old-spacewave", "staged-cli"} {
		copyFixtureExecutable(t, filepath.Join(statePath, name))
	}
	oldPath := filepath.Join(statePath, "old-spacewave")
	stagedPath := filepath.Join(statePath, "staged-cli")
	t.Setenv(sharedDaemonUpdateTarget, stagedPath)
	socketPath := filepath.Join(statePath, socketName)
	cleanupDaemonUpdateFixture(t, statePath)
	if err := daemon.StartExecutable(ctx, statePath, oldPath); err != nil {
		t.Fatal(err)
	}

	// Retain a real Resource watch while the launcher accepts the new CLI copy.
	connector := daemon.NewConnector(nil, nil)
	client, err := connector.Connect(ctx, statePath, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	atom := sharedDaemonAtom(t, client)
	stream, err := atom.WatchState(ctx, &resource_state.WatchStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	oldPID, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := client.Root().GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := rpc.ExecCall(ctx, "test.DaemonUpdate", "Accept",
		&desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON},
		&desktop_update.ApplyUpdateResponse{}); err != nil {
		t.Fatal(err)
	}
	if _, err := atom.SetState(ctx, &resource_state.SetStateRequest{StateJson: `"retained"`}); err != nil {
		t.Fatal(err)
	}
	if value, err := stream.Recv(); err != nil || value.GetStateJson() != `"retained"` {
		t.Fatalf("retained watch after acceptance: value=%v error=%v", value, err)
	}

	// A client arriving before the idle claim retains the old process.
	late, err := connector.Connect(ctx, statePath, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	client.Close()
	lateAtom := sharedDaemonAtom(t, late)
	if _, err := lateAtom.SetState(ctx, &resource_state.SetStateRequest{StateJson: `"late"`}); err != nil {
		t.Fatal(err)
	}
	if pid, err := os.ReadFile(filepath.Join(statePath, "runtime-identity")); err != nil || string(pid) != string(oldPID) {
		t.Fatalf("late client did not retain old daemon: pid=%q error=%v", pid, err)
	}
	late.Close()

	// The final close releases the owner claim, then readiness announces the
	// selected executable after the old state lease has been relinquished.
	for {
		select {
		case event := <-watcher.Events:
			if event.Name != socketPath+".ready" {
				continue
			}
			pid, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
			if err == nil && string(pid) != string(oldPID) {
				newClient, err := connector.Connect(ctx, statePath, socketPath)
				if err != nil {
					t.Fatal(err)
				}
				defer newClient.Close()
				executable, err := os.ReadFile(filepath.Join(statePath, "runtime-executable"))
				if err != nil || !strings.HasPrefix(string(executable), filepath.Join(statePath, "daemon-bin")+string(filepath.Separator)) {
					t.Fatalf("new daemon executable = %q, error = %v", executable, err)
				}
				return
			}
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-ctx.Done():
			entries, _ := os.ReadDir(statePath)
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
				if strings.HasPrefix(entry.Name(), "fixture-") && strings.HasSuffix(entry.Name(), ".log") {
					data, _ := os.ReadFile(filepath.Join(statePath, entry.Name()))
					t.Logf("%s: %s", entry.Name(), data)
				}
			}
			pid, _ := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
			t.Fatalf("updated daemon did not become ready: %v; current pid=%q; root entries=%v", ctx.Err(), pid, names)
		}
	}
}

// cleanupDaemonUpdateFixture shuts down the last controlled daemon process.
func cleanupDaemonUpdateFixture(t *testing.T, statePath string) {
	t.Helper()
	socketPath := filepath.Join(statePath, socketName)
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		stopWatcher, err := fsnotify.NewWatcher()
		if err != nil {
			t.Error(err)
			return
		}
		defer stopWatcher.Close()
		if err := stopWatcher.Add(statePath); err != nil {
			t.Error(err)
			return
		}
		pid, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
		if err != nil {
			t.Error(err)
			return
		}
		stoppedPath := filepath.Join(statePath, "stopped-"+string(pid))
		conn, err := (&net.Dialer{}).DialContext(stopCtx, "unix", socketPath)
		if err == nil {
			if err := requestDaemonShutdown(stopCtx, conn); err != nil {
				t.Error(err)
			}
			_ = conn.Close()
		}
		for {
			if _, err := os.Stat(stoppedPath); err == nil {
				return
			}
			select {
			case <-stopWatcher.Events:
			case err := <-stopWatcher.Errors:
				t.Error(err)
				return
			case <-stopCtx.Done():
				t.Error("fixture did not stop after cleanup: ", stopCtx.Err())
				return
			}
		}
	})
}

// copyFixtureExecutable creates one controlled copy of this test binary.
func copyFixtureExecutable(t *testing.T, destination string) {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
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
}
