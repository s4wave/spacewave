//go:build !skip_e2e && !js

package installedapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	installedAppPathEnv      = "SPACEWAVE_INSTALLED_APP_PATH"
	installedAppStateRootEnv = "SPACEWAVE_INSTALLED_APP_STATE_ROOT"
	startupTimeout           = 3 * time.Minute
)

// TIER: nightly
func TestPackagedInstalledAppRetainedStateLauncherStartupSmoke(t *testing.T) {
	if !E2EInstalledAppEnabled() {
		t.Skip("set ENABLE_E2E_INSTALLED_APP=true to run")
	}

	appPath := strings.TrimSpace(os.Getenv(installedAppPathEnv))
	if appPath == "" {
		t.Fatalf("set %s to a packaged installed Spacewave app path, e.g. /Applications/Spacewave.app", installedAppPathEnv)
	}

	executablePath, err := resolveInstalledAppExecutable(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" {
		if err := verifyDarwinCodeSignature(appPath); err != nil {
			t.Fatalf("verify installed app signature: %v", err)
		}
	}

	stateRoot, err := resolveInstalledAppStateRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stateRoot); err != nil {
		t.Fatalf("clear installed-app state root: %v", err)
	}
	t.Cleanup(func() {
		stopStateRootProcesses(stateRoot)
	})
	artifactDir := filepath.Join(stateRoot, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create installed-app artifact dir: %v", err)
	}

	initialProofs := []string{
		"initializing application and storage",
		"launcher starting",
		"starting electron:",
	}
	initial, err := runInstalledAppStartup(t, executablePath, stateRoot, artifactDir, "initial", initialProofs)
	if err != nil {
		t.Fatal(err)
	}
	retainedProofs := []string{
		"initializing application and storage",
		"starting electron:",
	}
	retained, err := runInstalledAppStartup(t, executablePath, stateRoot, artifactDir, "retained", retainedProofs)
	if err != nil {
		t.Fatal(err)
	}
	if initial.logPath == retained.logPath {
		t.Fatalf("expected distinct installed-app logs, got %q", initial.logPath)
	}

	breadcrumbPath, err := writeInstalledAppBreadcrumbs(
		artifactDir,
		appPath,
		executablePath,
		stateRoot,
		initial,
		retained,
	)
	if err != nil {
		t.Fatalf("write installed-app smoke breadcrumbs: %v", err)
	}
	t.Logf("installed-app retained startup smoke breadcrumbs: %s", breadcrumbPath)
}

type startupRun struct {
	name     string
	logPath  string
	tailPath string
	proofs   []string
}

func runInstalledAppStartup(
	t *testing.T,
	executablePath string,
	stateRoot string,
	artifactDir string,
	name string,
	proofs []string,
) (*startupRun, error) {
	t.Helper()

	logPath := filepath.Join(artifactDir, "installed-app-"+name+".log")
	spacewaveDataDir := filepath.Join(stateRoot, "spacewave-data")
	electronUserDataDir := filepath.Join(stateRoot, "electron-user-data")
	if err := os.MkdirAll(spacewaveDataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(electronUserDataDir, 0o755); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, executablePath)
	cmd.Dir = filepath.Dir(executablePath)
	cmd.Env = append(os.Environ(),
		"SPACEWAVE_DATA_DIR="+spacewaveDataDir,
		"BLDR_PLUGIN_STATE_PATH="+electronUserDataDir,
		"BLDR_LOG_LEVEL=debug",
		"BLDR_LOG_FILE=level=DEBUG;path="+logPath,
	)
	cmd.SysProcAttr = processGroupAttr()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start installed app %q: %w", executablePath, err)
	}
	defer func() {
		_ = stopProcessTree(cmd)
		stopStateRootProcesses(stateRoot)
	}()

	for _, proof := range proofs {
		if err := waitForLogSubstring(ctx, logPath, proof, startupTimeout); err != nil {
			return nil, fmt.Errorf("%s launch: %w", name, err)
		}
	}

	if err := stopProcessTree(cmd); err != nil {
		return nil, fmt.Errorf("stop installed app after %s launch: %w", name, err)
	}

	tailPath := filepath.Join(artifactDir, "installed-app-"+name+"-log-tail.txt")
	if err := writeLogTail(logPath, tailPath); err != nil {
		return nil, fmt.Errorf("write %s log tail: %w", name, err)
	}

	return &startupRun{
		name:     name,
		logPath:  logPath,
		tailPath: tailPath,
		proofs:   proofs,
	}, nil
}

func resolveInstalledAppExecutable(appPath string) (string, error) {
	info, err := os.Stat(appPath)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" && info.IsDir() && strings.HasSuffix(appPath, ".app") {
		exe := filepath.Join(appPath, "Contents", "MacOS", "Spacewave")
		if _, err := os.Stat(exe); err != nil {
			return "", err
		}
		return exe, nil
	}
	if info.IsDir() {
		return "", fmt.Errorf("installed app path is a directory but not a supported app bundle: %s", appPath)
	}
	return appPath, nil
}

func resolveInstalledAppStateRoot() (string, error) {
	stateRoot := strings.TrimSpace(os.Getenv(installedAppStateRootEnv))
	if stateRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateRoot = filepath.Join(home, "sw-e2e-installed")
	}
	stateRoot = filepath.Clean(stateRoot)
	if stateRoot == "." || stateRoot == string(filepath.Separator) {
		return "", fmt.Errorf("%s must not be %q", installedAppStateRootEnv, stateRoot)
	}
	return stateRoot, nil
}

func verifyDarwinCodeSignature(appPath string) error {
	target := darwinAppBundleRoot(appPath)
	cmd := exec.Command("codesign", "--verify", "--deep", "--strict", target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func darwinAppBundleRoot(path string) string {
	orig := filepath.Clean(path)
	path = filepath.Clean(path)
	for {
		if strings.HasSuffix(path, ".app") {
			return path
		}
		next := filepath.Dir(path)
		if next == path {
			return orig
		}
		path = next
	}
}

func waitForLogSubstring(ctx context.Context, path string, want string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var last string
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			last = string(data)
			if strings.Contains(last, want) {
				return nil
			}
		}

		select {
		case <-waitCtx.Done():
			if len(last) > 4000 {
				last = last[len(last)-4000:]
			}
			return fmt.Errorf("timeout waiting for log substring %q in %s\nlast log tail:\n%s", want, path, last)
		case <-ticker.C:
		}
	}
}

func writeInstalledAppBreadcrumbs(
	artifactDir string,
	appPath string,
	executablePath string,
	stateRoot string,
	initial *startupRun,
	retained *startupRun,
) (string, error) {
	breadcrumbPath := filepath.Join(artifactDir, "installed-app-retained-startup-breadcrumbs.txt")
	lines := []string{
		"smoke=packaged-installed-app-retained-state-launcher-startup",
		"installed_app_path=" + appPath,
		"executable_path=" + executablePath,
		"state_root=" + stateRoot,
		"spacewave_data_dir=" + filepath.Join(stateRoot, "spacewave-data"),
		"electron_user_data_dir=" + filepath.Join(stateRoot, "electron-user-data"),
		"initial_log=" + initial.logPath,
		"retained_log=" + retained.logPath,
		"initial_log_tail=" + initial.tailPath,
		"retained_log_tail=" + retained.tailPath,
		"proof=initial_installed_app_launcher_started",
		"proof=retained_installed_app_electron_started",
		"proof=retained_state_root_reused",
	}
	if runtime.GOOS == "darwin" {
		lines = append(lines, "proof=installed_app_signature_verified")
	}
	for _, run := range []*startupRun{initial, retained} {
		for _, proof := range run.proofs {
			lines = append(lines, "proof="+run.name+"_log:"+proof)
		}
	}
	lines = append(lines,
		"update_breadcrumb=initial packaged installed app launch populated launcher state",
		"update_breadcrumb=retained packaged installed app launch reused the same app data and Electron user data roots",
		"update_breadcrumb=retained packaged installed app launch reached Electron startup instead of hanging in retained state",
		"",
	)
	if err := os.WriteFile(breadcrumbPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return breadcrumbPath, nil
}

func writeLogTail(srcPath string, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	const maxTailBytes = 32 * 1024
	if len(data) > maxTailBytes {
		data = data[len(data)-maxTailBytes:]
	}
	return os.WriteFile(dstPath, data, 0o644)
}
