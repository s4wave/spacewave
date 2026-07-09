//go:build !skip_e2e && !js

package installedapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/gitroot"
)

const (
	desktopDistributionGateEnv           = "ENABLE_E2E_DESKTOP_DISTRIBUTION_GATE"
	desktopDistributionMinFreeBytesEnv   = "E2E_DESKTOP_DISTRIBUTION_MIN_FREE_BYTES"
	desktopDistributionE2EVersion        = "0.0.0"
	defaultDesktopDistributionMinFreeMem = int64(3 * 1024 * 1024 * 1024)
	desktopDistributionBuildTimeout      = 90 * time.Minute
	desktopDistributionReadyTimeout      = 3 * time.Minute
	desktopDistributionShutdownTimeout   = 45 * time.Second
)

type desktopDistributionPackage struct {
	AppPath        string
	ExecutablePath string
	BuildLogPath   string
	BuildTailPath  string
}

var desktopDistributionPackageCache struct {
	sync.Mutex
	pkg *desktopDistributionPackage
}

type desktopDistributionVMStat struct {
	PagesFree        int64
	PagesInactive    int64
	PagesSpeculative int64
	PageSize         int64
	Raw              string
}

type desktopDistributionRun struct {
	Name          string
	PID           int
	LogPath       string
	TailPath      string
	StatusText    string
	ListenerLabel string
	SocketPath    string
}

// TIER: nightly
func TestMacOSPackagedDesktopDistributionLifecycleGate(t *testing.T) {
	if !desktopDistributionGateEnabled() {
		t.Skipf("set %s=true to run the macOS desktop distribution lifecycle gate; current GOOS=%s", desktopDistributionGateEnv, runtime.GOOS)
	}
	if runtime.GOOS != "darwin" {
		t.Skipf("macOS desktop distribution gate only runs on darwin; got %s", runtime.GOOS)
	}

	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	artifactDir, err := os.MkdirTemp("", "spacewave-desktop-distribution-artifacts-*")
	if err != nil {
		t.Fatalf("create desktop distribution artifact dir: %v", err)
	}
	t.Logf("desktop distribution gate artifacts: %s", artifactDir)

	vmStat, minFreeBytes, err := checkDesktopDistributionFreeMemory(artifactDir)
	if err != nil {
		t.Fatalf("check macOS free memory before packaging: %v", err)
	}
	freeBytes := vmStat.FreeableBytes()
	t.Logf("vm_stat Pages free=%d inactive=%d speculative=%d page_size=%d freeable_bytes=%d min_free_bytes=%d", vmStat.PagesFree, vmStat.PagesInactive, vmStat.PagesSpeculative, vmStat.PageSize, freeBytes, minFreeBytes)
	if freeBytes < minFreeBytes {
		t.Skipf("vm_stat freeable memory %d bytes is below %s=%d; skipping desktop distribution build on shared Mac", freeBytes, desktopDistributionMinFreeBytesEnv, minFreeBytes)
	}

	stateRoot, err := createDesktopDistributionStateRoot()
	if err != nil {
		t.Fatalf("create desktop distribution state root: %v", err)
	}
	t.Cleanup(func() {
		stopStateRootProcesses(stateRoot)
		if err := os.RemoveAll(stateRoot); err != nil {
			t.Logf("remove desktop distribution state root %s: %v", stateRoot, err)
		}
	})

	pkg, err := getDesktopDistributionPackagedApp(t, repoRoot, artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	appPath := pkg.AppPath
	executablePath := pkg.ExecutablePath
	buildLogPath := pkg.BuildLogPath
	buildTailPath := pkg.BuildTailPath

	sentinelPath := filepath.Join(stateRoot, "spacewave-data", "desktop-distribution-sentinel.txt")
	sentinelBody := "spacewave desktop distribution gate sentinel\n"
	initial, err := runDesktopDistributionLaunch(t, executablePath, stateRoot, artifactDir, "initial", func() error {
		return os.WriteFile(sentinelPath, []byte(sentinelBody), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	relaunched, err := runDesktopDistributionLaunch(t, executablePath, stateRoot, artifactDir, "relaunch", nil)
	if err != nil {
		t.Fatal(err)
	}
	gotSentinel, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("read persisted sentinel after relaunch: %v", err)
	}
	if string(gotSentinel) != sentinelBody {
		t.Fatalf("sentinel contents changed after relaunch: got %q", gotSentinel)
	}

	breadcrumbPath, err := writeDesktopDistributionBreadcrumbs(
		artifactDir,
		appPath,
		executablePath,
		stateRoot,
		sentinelPath,
		buildLogPath,
		buildTailPath,
		vmStat,
		minFreeBytes,
		initial,
		relaunched,
	)
	if err != nil {
		t.Fatalf("write desktop distribution breadcrumbs: %v", err)
	}
	t.Logf("desktop distribution lifecycle breadcrumbs: %s", breadcrumbPath)
}

func desktopDistributionGateEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(desktopDistributionGateEnv)), "true")
}

func createDesktopDistributionStateRoot() (string, error) {
	// macOS caps Unix socket paths at 104 bytes; plugin host sockets live under
	// SPACEWAVE_DATA_DIR, so use a deliberately short temp root.
	return os.MkdirTemp("/tmp", "swdg-*")
}

func checkDesktopDistributionFreeMemory(artifactDir string) (*desktopDistributionVMStat, int64, error) {
	minFreeBytes, err := resolveDesktopDistributionMinFreeBytes()
	if err != nil {
		return nil, 0, err
	}
	cmd := exec.Command("vm_stat")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 0, fmt.Errorf("vm_stat: %w: %s", err, strings.TrimSpace(string(out)))
	}
	vmStatPath := filepath.Join(artifactDir, "vm_stat.txt")
	if err := os.WriteFile(vmStatPath, out, 0o644); err != nil {
		return nil, 0, fmt.Errorf("write vm_stat artifact: %w", err)
	}
	stat, err := parseVMStat(string(out))
	if err != nil {
		return nil, 0, err
	}
	return stat, minFreeBytes, nil
}

func resolveDesktopDistributionMinFreeBytes() (int64, error) {
	raw := strings.TrimSpace(os.Getenv(desktopDistributionMinFreeBytesEnv))
	if raw == "" {
		return defaultDesktopDistributionMinFreeMem, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s as bytes: %w", desktopDistributionMinFreeBytesEnv, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("%s must be non-negative bytes, got %d", desktopDistributionMinFreeBytesEnv, v)
	}
	return v, nil
}

func (s *desktopDistributionVMStat) FreeableBytes() int64 {
	return (s.PagesFree + s.PagesInactive + s.PagesSpeculative) * s.PageSize
}

func parseVMStat(raw string) (*desktopDistributionVMStat, error) {
	var pageSize int64
	var pagesFree int64
	var pagesInactive int64
	var pagesSpeculative int64
	for line := range strings.SplitSeq(raw, "\n") {
		if strings.Contains(line, "page size of ") {
			pageSizeStart := strings.Index(line, "page size of ")
			pageSizeEnd := strings.Index(line[pageSizeStart:], " bytes")
			if pageSizeEnd >= 0 {
				pageSizeRaw := line[pageSizeStart+len("page size of ") : pageSizeStart+pageSizeEnd]
				parsed, err := strconv.ParseInt(strings.TrimSpace(pageSizeRaw), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("parse vm_stat page size %q: %w", pageSizeRaw, err)
				}
				pageSize = parsed
			}
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Pages free:") {
			parsed, err := parseVMStatPageCount(trimmed, "Pages free:")
			if err != nil {
				return nil, err
			}
			pagesFree = parsed
		}
		if strings.HasPrefix(trimmed, "Pages inactive:") {
			parsed, err := parseVMStatPageCount(trimmed, "Pages inactive:")
			if err != nil {
				return nil, err
			}
			pagesInactive = parsed
		}
		if strings.HasPrefix(trimmed, "Pages speculative:") {
			parsed, err := parseVMStatPageCount(trimmed, "Pages speculative:")
			if err != nil {
				return nil, err
			}
			pagesSpeculative = parsed
		}
	}
	if pageSize <= 0 {
		return nil, errors.New("vm_stat output did not include a positive page size")
	}
	if pagesFree <= 0 {
		return nil, errors.New("vm_stat output did not include positive Pages free")
	}
	return &desktopDistributionVMStat{
		PagesFree:        pagesFree,
		PagesInactive:    pagesInactive,
		PagesSpeculative: pagesSpeculative,
		PageSize:         pageSize,
		Raw:              raw,
	}, nil
}

func parseVMStatPageCount(line string, prefix string) (int64, error) {
	pagesRaw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	pagesRaw = strings.TrimSuffix(pagesRaw, ".")
	pagesRaw = strings.ReplaceAll(pagesRaw, ",", "")
	parsed, err := strconv.ParseInt(strings.TrimSpace(pagesRaw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse vm_stat %s %q: %w", strings.TrimSuffix(prefix, ":"), pagesRaw, err)
	}
	return parsed, nil
}

func runDesktopDistributionPackaging(t *testing.T, repoRoot string, artifactDir string) (string, string, error) {
	t.Helper()
	logPath := filepath.Join(artifactDir, "macos-packaging.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", "", fmt.Errorf("create packaging log: %w", err)
	}
	defer logFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), desktopDistributionBuildTimeout)
	defer cancel()

	if err := cleanDesktopDistributionReleaseOutputs(repoRoot); err != nil {
		return logPath, "", err
	}
	signingEnv := []string{
		"BLDR_MACOS_ADHOC_SIGN=1",
		"BLDR_MACOS_SIGN_IDENTITY=-",
		"BLDR_MACOS_SIGN_TEAM_ID=ADHOCSPACE",
	}
	commands := []struct {
		name string
		args []string
		env  []string
	}{
		{
			name: "generate desktop metadata",
			args: []string{"bash", "scripts/release/gen-desktop.sh"},
		},
		{
			name: "generate icons",
			args: []string{"bash", "scripts/release/gen-icons.sh", filepath.Join(repoRoot, "web", "images", "spacewave-icon.png")},
		},
		{
			name: "build macOS helpers",
			args: []string{"bash", "scripts/release/build-helper.sh", "darwin", runtime.GOARCH},
		},
		{
			name: "build remote web entrypoint",
			args: []string{"go", "run", "github.com/s4wave/spacewave/bldr/cmd/bldr", "--build-type=release", "build", "-b", "release-remote-web"},
		},
		{
			name: "build remote js entrypoint",
			args: []string{"go", "run", "github.com/s4wave/spacewave/bldr/cmd/bldr", "--build-type=release", "build", "-b", "release-remote-js"},
		},
		{
			name: "build desktop entrypoint",
			args: []string{"go", "run", "github.com/s4wave/spacewave/bldr/cmd/bldr", "--build-type=release", "build", "-b", "release-desktop-darwin-" + runtime.GOARCH},
			env:  signingEnv,
		},
	}
	for _, command := range commands {
		t.Logf("desktop distribution packaging: %s", command.name)
		if err := runDesktopDistributionPackagingCommand(ctx, repoRoot, logFile, command.name, command.args, command.env); err != nil {
			return logPath, "", fmt.Errorf("%w\n%s", err, readTailForError(logPath))
		}
	}
	t.Log("desktop distribution packaging: stage runtime binary")
	if err := stageDesktopDistributionRuntimeBinary(repoRoot); err != nil {
		return logPath, "", fmt.Errorf("stage desktop runtime binary: %w\n%s", err, readTailForError(logPath))
	}
	t.Log("desktop distribution packaging: package macOS app")
	if err := runDesktopDistributionPackagingCommand(
		ctx,
		repoRoot,
		logFile,
		"package macOS app",
		[]string{"bash", "scripts/release/build-macos.sh", runtime.GOARCH, desktopDistributionE2EVersion, "--skip-notarize"},
		signingEnv,
	); err != nil {
		return logPath, "", fmt.Errorf("%w\n%s", err, readTailForError(logPath))
	}

	tailPath := filepath.Join(artifactDir, "macos-packaging-tail.txt")
	if err := writeLogTail(logPath, tailPath); err != nil {
		return logPath, "", fmt.Errorf("write packaging log tail: %w", err)
	}
	return logPath, tailPath, nil
}

func cleanDesktopDistributionReleaseOutputs(repoRoot string) error {
	for _, rel := range []string{
		filepath.Join(".tmp", "dist"),
		filepath.Join(".tmp", "Spacewave.app"),
		filepath.Join(".tmp", "Spacewave-amd64.zip"),
		filepath.Join(".tmp", "Spacewave-arm64.zip"),
		filepath.Join(".tmp", "dmg-rw-"+runtime.GOARCH+".dmg"),
		filepath.Join(".tmp", "dmg-stage-"+runtime.GOARCH),
		filepath.Join(".tmp", "macos-helper-plists"),
		"staging",
		filepath.Join("dist", "installers"),
	} {
		if err := os.RemoveAll(filepath.Join(repoRoot, rel)); err != nil {
			return fmt.Errorf("clean %s: %w", rel, err)
		}
	}
	return nil
}

func runDesktopDistributionPackagingCommand(
	ctx context.Context,
	repoRoot string,
	logFile *os.File,
	name string,
	args []string,
	env []string,
) error {
	if len(args) == 0 {
		return errors.New("packaging command has no args")
	}
	if _, err := fmt.Fprintf(logFile, "\n=== %s: %s ===\n", name, strings.Join(args, " ")); err != nil {
		return err
	}
	// #nosec G204 -- the package gate runs only the fixed command table above.
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func stageDesktopDistributionRuntimeBinary(repoRoot string) error {
	srcBin := filepath.Join(
		repoRoot,
		".bldr",
		"build",
		"desktop",
		"darwin",
		runtime.GOARCH,
		"spacewave-dist",
		"dist",
		"spacewave",
	)
	dstDir := filepath.Join(repoRoot, ".tmp", "dist", "darwin-"+runtime.GOARCH)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	dstBin := filepath.Join(dstDir, "spacewave")
	if err := copyDesktopDistributionFile(srcBin, dstBin); err != nil {
		return err
	}
	return os.Chmod(dstBin, 0o755)
}

func copyDesktopDistributionFile(srcPath string, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

func getDesktopDistributionPackagedApp(t *testing.T, repoRoot string, artifactDir string) (*desktopDistributionPackage, error) {
	t.Helper()
	desktopDistributionPackageCache.Lock()
	defer desktopDistributionPackageCache.Unlock()

	if desktopDistributionPackageCache.pkg != nil {
		if _, err := os.Stat(desktopDistributionPackageCache.pkg.ExecutablePath); err == nil {
			if err := verifyDarwinCodeSignature(desktopDistributionPackageCache.pkg.AppPath); err != nil {
				return nil, fmt.Errorf("verify cached packaged app signature: %w", err)
			}
			return desktopDistributionPackageCache.pkg, nil
		}
		desktopDistributionPackageCache.pkg = nil
	}

	buildLogPath, buildTailPath, err := runDesktopDistributionPackaging(t, repoRoot, artifactDir)
	if err != nil {
		return nil, err
	}
	appPath := filepath.Join(repoRoot, ".tmp", "Spacewave.app")
	executablePath, err := resolveInstalledAppExecutable(appPath)
	if err != nil {
		return nil, fmt.Errorf("resolve packaged app executable: %w", err)
	}
	if err := verifyDarwinCodeSignature(appPath); err != nil {
		return nil, fmt.Errorf("verify packaged app signature: %w", err)
	}
	desktopDistributionPackageCache.pkg = &desktopDistributionPackage{
		AppPath:        appPath,
		ExecutablePath: executablePath,
		BuildLogPath:   buildLogPath,
		BuildTailPath:  buildTailPath,
	}
	return desktopDistributionPackageCache.pkg, nil
}

func runDesktopDistributionLaunch(
	t *testing.T,
	executablePath string,
	stateRoot string,
	artifactDir string,
	name string,
	afterReady func() error,
) (*desktopDistributionRun, error) {
	t.Helper()

	spacewaveDataDir := filepath.Join(stateRoot, "spacewave-data")
	electronUserDataDir := filepath.Join(stateRoot, "electron-user-data")
	if err := os.MkdirAll(spacewaveDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create SPACEWAVE_DATA_DIR: %w", err)
	}
	if err := os.MkdirAll(electronUserDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create BLDR_PLUGIN_STATE_PATH: %w", err)
	}

	logPath := filepath.Join(artifactDir, "desktop-distribution-"+name+".log")
	tailPath := filepath.Join(artifactDir, "desktop-distribution-"+name+"-tail.txt")
	defer func() {
		if err := writeLogTailIfPresent(logPath, tailPath); err != nil {
			t.Logf("write %s launch log tail: %v", name, err)
		}
	}()

	cmd := exec.Command(executablePath)
	cmd.Dir = filepath.Dir(executablePath)
	cmd.Env = append(os.Environ(),
		"SPACEWAVE_DATA_DIR="+spacewaveDataDir,
		"BLDR_PLUGIN_STATE_PATH="+electronUserDataDir,
		"BLDR_LOG_LEVEL=debug",
		"BLDR_LOG_FILE=level=DEBUG;path="+logPath,
	)
	cmd.SysProcAttr = processGroupAttr()

	waitStarted := false
	defer func() {
		if !waitStarted {
			_ = stopProcessTree(cmd)
		}
		stopStateRootProcesses(stateRoot)
	}()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start packaged desktop app %q: %w", executablePath, err)
	}
	pid := cmd.Process.Pid

	readyCtx, readyCancel := context.WithTimeout(context.Background(), desktopDistributionReadyTimeout)
	defer readyCancel()
	readyMarker, err := waitForDesktopDistributionLogReady(readyCtx, logPath)
	if err != nil {
		return nil, fmt.Errorf("%s launch wait for packaged renderer ready: %w\n%s", name, err, readTailForError(logPath))
	}
	if afterReady != nil {
		if err := afterReady(); err != nil {
			return nil, fmt.Errorf("%s launch after-ready hook: %w", name, err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), desktopDistributionShutdownTimeout)
	defer shutdownCancel()
	waitStarted = true
	if err := stopProcessTree(cmd); err != nil {
		return nil, fmt.Errorf("%s launch stop packaged app: %w", name, err)
	}
	if err := waitForDesktopDistributionProcessGroupEmpty(shutdownCtx, pid); err != nil {
		return nil, fmt.Errorf("%s launch process group orphan check: %w", name, err)
	}
	if err := writeLogTail(logPath, tailPath); err != nil {
		return nil, fmt.Errorf("write %s launch log tail: %w", name, err)
	}

	return &desktopDistributionRun{
		Name:          name,
		PID:           pid,
		LogPath:       logPath,
		TailPath:      tailPath,
		StatusText:    readyMarker,
		ListenerLabel: "packaged-renderer-log",
		SocketPath:    spacewaveDataDir,
	}, nil
}

func waitForDesktopDistributionLogReady(ctx context.Context, logPath string) (string, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		data, err := os.ReadFile(logPath)
		if err == nil {
			rendererLoaded := bytes.Contains(data, []byte("appRequestHandler: forwarding Bldr request: /b/pa/spacewave-app/"))
			appAssetsServed := bytes.Contains(data, []byte("accessing plugin assets filesystem")) &&
				bytes.Contains(data, []byte("plugin-id=spacewave-app"))
			if rendererLoaded && appAssetsServed {
				return "packaged renderer loaded and served spacewave app assets", nil
			}
		} else if !os.IsNotExist(err) {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return "", fmt.Errorf("%w; last log read error: %v", ctx.Err(), lastErr)
			}
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForDesktopDistributionProcessGroupEmpty(ctx context.Context, pid int) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !processGroupAlive(pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w; process group %d still exists", ctx.Err(), pid)
		case <-ticker.C:
		}
	}
}

func writeLogTailIfPresent(srcPath string, dstPath string) error {
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return writeLogTail(srcPath, dstPath)
}

func readTailForError(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unable to read log tail: " + err.Error()
	}
	const maxTailBytes = 8192
	if len(data) > maxTailBytes {
		data = data[len(data)-maxTailBytes:]
	}
	return string(data)
}

func writeDesktopDistributionBreadcrumbs(
	artifactDir string,
	appPath string,
	executablePath string,
	stateRoot string,
	sentinelPath string,
	buildLogPath string,
	buildTailPath string,
	vmStat *desktopDistributionVMStat,
	minFreeBytes int64,
	initial *desktopDistributionRun,
	relaunched *desktopDistributionRun,
) (string, error) {
	breadcrumbPath := filepath.Join(artifactDir, "desktop-distribution-lifecycle-breadcrumbs.txt")
	lines := []string{
		"gate=macos-packaged-desktop-distribution-lifecycle",
		"env_gate=" + desktopDistributionGateEnv,
		"version=" + desktopDistributionE2EVersion,
		"platform=darwin-" + runtime.GOARCH,
		"app_path=" + appPath,
		"executable_path=" + executablePath,
		"state_root=" + stateRoot,
		"spacewave_data_dir=" + filepath.Join(stateRoot, "spacewave-data"),
		"electron_user_data_dir=" + filepath.Join(stateRoot, "electron-user-data"),
		"sentinel_path=" + sentinelPath,
		"packaging_log=" + buildLogPath,
		"packaging_log_tail=" + buildTailPath,
		"vm_stat_pages_free=" + strconv.FormatInt(vmStat.PagesFree, 10),
		"vm_stat_pages_inactive=" + strconv.FormatInt(vmStat.PagesInactive, 10),
		"vm_stat_pages_speculative=" + strconv.FormatInt(vmStat.PagesSpeculative, 10),
		"vm_stat_page_size=" + strconv.FormatInt(vmStat.PageSize, 10),
		"vm_stat_freeable_bytes=" + strconv.FormatInt(vmStat.FreeableBytes(), 10),
		"vm_stat_min_free_bytes=" + strconv.FormatInt(minFreeBytes, 10),
		"proof=release_macos_package_script_packaged_darwin_app",
		"proof=packaged_app_signature_verified",
		"proof=initial_packaged_renderer_loaded_web_manifest",
		"proof=initial_signal_shutdown_left_no_process_group_orphan",
		"proof=relaunch_packaged_renderer_loaded_web_manifest",
		"proof=relaunch_signal_shutdown_left_no_process_group_orphan",
		"proof=state_root_sentinel_preserved_after_relaunch",
	}
	for _, run := range []*desktopDistributionRun{initial, relaunched} {
		lines = append(lines,
			run.Name+"_pid="+strconv.Itoa(run.PID),
			run.Name+"_ready_marker="+run.StatusText,
			run.Name+"_listener_label="+run.ListenerLabel,
			run.Name+"_state_path="+run.SocketPath,
			run.Name+"_log="+run.LogPath,
			run.Name+"_log_tail="+run.TailPath,
		)
	}
	lines = append(lines, "")
	if err := os.WriteFile(breadcrumbPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return breadcrumbPath, nil
}
