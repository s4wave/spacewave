//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fsnotify"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	"github.com/s4wave/spacewave/core/daemon"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// TestDaemonUpdateChangedAfterAcceptanceReportsError keeps admission open and
// permits an explicit retry after the selected fixture bytes are restored.
func TestDaemonUpdateChangedAfterAcceptanceReportsError(t *testing.T) {
	statePath := shortSocketDir(t)
	oldPath := filepath.Join(statePath, "old-spacewave")
	stagedPath := filepath.Join(statePath, "staged-cli")
	copyFixtureExecutable(t, oldPath)
	copyFixtureExecutable(t, stagedPath)
	t.Setenv(sharedDaemonFixtureMode, "native")
	t.Setenv(sharedDaemonUpdateTarget, stagedPath)
	t.Setenv(sharedDaemonCorruptOnAccept, "1")
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
	cleanupDaemonUpdateFixture(t, statePath)
	if err := daemon.StartExecutable(ctx, statePath, oldPath); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(statePath, socketName)
	connector := daemon.NewConnector(nil, nil)
	client, err := connector.Connect(ctx, statePath, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	oldPID, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := client.Root().GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		if err := rpc.ExecCall(ctx, "test.DaemonUpdate", "Accept",
			&desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON},
			&desktop_update.ApplyUpdateResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	apply()
	for {
		select {
		case event := <-watcher.Events:
			if event.Name == filepath.Join(statePath, "update-failed") {
				goto failureReported
			}
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("accepted artifact failure was not reported: ", ctx.Err())
		}
	}

failureReported:
	status := &spacewave_launcher.LauncherInfo{}
	if err := rpc.ExecCall(ctx, "test.DaemonUpdate", "Status", &spacewave_launcher.WatchLauncherInfoRequest{}, status); err != nil {
		t.Fatal(err)
	}
	state := status.GetDaemonUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR || !strings.Contains(state.GetErrorMessage(), "digest") {
		t.Fatalf("visible daemon failure = %v", state)
	}
	late, err := connector.Connect(ctx, statePath, socketPath)
	if err != nil {
		t.Fatalf("admission was fenced after preparation failure: %v", err)
	}
	if pid, err := os.ReadFile(filepath.Join(statePath, "runtime-identity")); err != nil || string(pid) != string(oldPID) {
		t.Fatalf("old daemon stopped after preparation failure: pid=%q error=%v", pid, err)
	}
	if err := rpc.ExecCall(ctx, "test.DaemonUpdate", "Restore",
		&spacewave_launcher.WatchLauncherInfoRequest{}, &spacewave_launcher.WatchLauncherInfoRequest{}); err != nil {
		t.Fatal(err)
	}
	apply()
	late.Close()
	client.Close()

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
				newClient.Close()
				return
			}
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("retry did not relaunch daemon: ", ctx.Err())
		}
	}
}
