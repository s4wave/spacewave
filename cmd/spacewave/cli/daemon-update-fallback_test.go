//go:build !js && !windows

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aperturerobotics/fsnotify"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	"github.com/s4wave/spacewave/core/daemon"
)

// TestDaemonUpdateFailedReadinessRestoresOldCLI proves the old lease releases
// before fallback readiness and the failed selected child leaves no service.
func TestDaemonUpdateFailedReadinessRestoresOldCLI(t *testing.T) {
	statePath := shortSocketDir(t)
	oldPath := filepath.Join(statePath, "old-spacewave")
	stagedPath := filepath.Join(statePath, "failed-cli.sh")
	copyFixtureExecutable(t, oldPath)
	copyFixtureExecutable(t, stagedPath)
	t.Setenv(sharedDaemonFixtureMode, "native")
	t.Setenv(sharedDaemonUpdateTarget, stagedPath)
	t.Setenv(sharedDaemonCorruptOnAccept, "")
	t.Setenv(sharedDaemonFailSelected, "1")
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)
	t.Setenv("SPACEWAVE_SOCKET_PATH", "")
	t.Setenv(daemonIdleTimeoutEnvVar, "30s")
	t.Setenv(daemon.StartupTimeoutEnvVar, "5s")
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
	client.Close()

	for {
		select {
		case event := <-watcher.Events:
			if event.Name != socketPath+".ready" {
				continue
			}
			pid, err := os.ReadFile(filepath.Join(statePath, "runtime-identity"))
			if err != nil || string(pid) == string(oldPID) {
				continue
			}
			failedPIDBytes, err := os.ReadFile(filepath.Join(statePath, "failed-new-pid"))
			if err != nil {
				t.Fatalf("selected executable did not attempt startup: %v", err)
			}
			failedPID, err := strconv.Atoi(string(failedPIDBytes))
			if err != nil {
				t.Fatal(err)
			}
			if err := syscall.Kill(failedPID, 0); err != syscall.ESRCH {
				t.Fatalf("failed selected child was not joined: pid=%d error=%v", failedPID, err)
			}
			executable, err := os.ReadFile(filepath.Join(statePath, "runtime-executable"))
			if err != nil || !strings.HasPrefix(string(executable), filepath.Join(statePath, "daemon-bin")+string(filepath.Separator)) || strings.HasSuffix(string(executable), ".sh") {
				t.Fatalf("fallback executable = %q, error = %v", executable, err)
			}
			fallbackClient, err := connector.Connect(ctx, statePath, socketPath)
			if err != nil {
				t.Fatalf("fallback daemon did not serve Resource: %v", err)
			}
			fallbackClient.Close()
			return
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("fallback daemon did not become ready: ", ctx.Err())
		}
	}
}
