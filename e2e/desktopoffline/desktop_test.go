//go:build !js

package desktopoffline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/fsnotify"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	"github.com/s4wave/spacewave/core/daemon"
)

// TestDesktopDistributionSocket opens the built fixture through the protected
// daemon socket. Build preparation is deliberately separate from this short test.
func TestDesktopDistributionSocket(t *testing.T) {
	// Require the prepared desktop fixture before creating its isolated state.
	bin := os.Getenv("SPACEWAVE_OFFLINE_DESKTOP_BIN")
	if bin == "" {
		t.Skip("build with e2e/desktopoffline/build.sh and set SPACEWAVE_OFFLINE_DESKTOP_BIN")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	statePath, err := os.MkdirTemp(filepath.Join(root, ".tmp"), "sd-")
	if err != nil {
		t.Fatal(err)
	}

	// Watch the fixture state directory for daemon readiness changes.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Add(statePath); err != nil {
		t.Fatal(err)
	}

	// Capture the desktop fixture daemon output for failure diagnosis.
	logFile, err := os.Create(filepath.Join(statePath, "serve.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	// Start the desktop fixture daemon with a bounded test lifetime.
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Second)
	defer cancel()
	cmd := exec.Command(bin, "--state-path", statePath, "serve", "--idle-timeout=0")
	cmd.Env = append(os.Environ(),
		"SPACEWAVE_STATE_PATH="+statePath,
		"SPACEWAVE_DATA_DIR="+statePath,
		"BLDR_PLUGIN_STATE_PATH="+filepath.Join(statePath, "electron"),
		"BLDR_LOG_FILE=level=DEBUG;path="+filepath.Join(statePath, "daemon.log"),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Observe daemon exit and arrange process cleanup after the test.
	done := make(chan struct{})
	var processErr error
	go func() {
		processErr = cmd.Wait()
		close(done)
	}()
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	// Wait for the daemon readiness marker or its terminal failure.
	readyPath := filepath.Join(statePath, daemon.SocketName) + ".ready"
	for {
		ready, err := os.ReadFile(readyPath)
		if err == nil && string(ready) == "ready\n" {
			break
		}
		select {
		case <-watcher.Events:
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-done:
			t.Fatalf("desktop fixture daemon exited before readiness: %v; log: %s", processErr, logFile.Name())
		case <-ctx.Done():
			t.Fatalf("desktop fixture daemon did not become ready: %v; log: %s", ctx.Err(), logFile.Name())
		}
	}

	// Connect to the ready fixture daemon over its protected socket.
	client, err := daemon.Connect(ctx, statePath, filepath.Join(statePath, daemon.SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Verify the fixture exposes its local provider and session listing.
	providers, err := client.Root().ListProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	localSeen := false
	for _, provider := range providers {
		if provider.GetProviderId() == "spacewave" {
			t.Fatalf("fixture exposed a project cloud provider: %s", provider.GetProviderId())
		}
		if provider.GetProviderId() == "local" {
			localSeen = true
		}
	}
	if !localSeen {
		t.Fatal("fixture did not mount the local provider")
	}
	if _, err := client.Root().ListSessions(ctx); err != nil {
		t.Fatal(err)
	}

	// Open the desktop and verify the response identifies the fixture daemon and UI.
	opened, err := desktop_control.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(
		ctx,
		&desktop_control.OpenOrFocusDesktopRequest{},
	)
	if err != nil {
		t.Fatalf("open desktop: %v; log: %s", err, logFile.Name())
	}
	if opened.GetDaemonPid() != int64(cmd.Process.Pid) || opened.GetUiManifestRef() == "" {
		t.Fatalf("desktop response has wrong daemon or UI artifact: %v", opened)
	}
	t.Logf("daemon %d opened UI manifest %s", opened.GetDaemonPid(), opened.GetUiManifestRef())
}
