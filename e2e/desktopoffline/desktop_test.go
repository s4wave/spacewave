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
	"github.com/s4wave/spacewave/core/daemon"
	desktopcontrol "github.com/s4wave/spacewave/core/daemon/desktopcontrol"
)

// TestDesktopDistributionSocket opens the built fixture through the protected
// daemon socket. Build preparation is deliberately separate from this short test.
func TestDesktopDistributionSocket(t *testing.T) {
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
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Add(statePath); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(statePath, "serve.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

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

	client, err := daemon.Connect(ctx, statePath, filepath.Join(statePath, daemon.SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
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

	opened, err := desktopcontrol.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(
		ctx,
		&desktopcontrol.OpenOrFocusDesktopRequest{},
	)
	if err != nil {
		t.Fatalf("open desktop: %v; log: %s", err, logFile.Name())
	}
	if opened.GetDaemonPid() != int64(cmd.Process.Pid) || opened.GetUiManifestRef() == "" {
		t.Fatalf("desktop response has wrong daemon or UI artifact: %v", opened)
	}
	t.Logf("daemon %d opened UI manifest %s", opened.GetDaemonPid(), opened.GetUiManifestRef())
}
