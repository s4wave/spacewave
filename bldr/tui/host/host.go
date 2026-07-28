//go:build !js && !windows

package bldr_tui_host

import (
	"bufio"
	"context"
	"embed"
	stderrors "errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

const (
	readyMarker       = "TUI_READY"
	terminalRestore   = "\x1b[0m\x1b[?25h\x1b[?1049l"
	childExitDeadline = 2 * time.Second
)

//go:embed host-loader.ts
var loaderFS embed.FS

// Host owns the Bun child, private Resource proxy, readiness, restart, and terminal cleanup.
type Host struct {
	config Config
}

// NewHost constructs a generic Bun TuiView host.
func NewHost(config Config) (*Host, error) {
	if strings.TrimSpace(config.BunPath) == "" {
		config.BunPath = "bun"
	}
	if strings.TrimSpace(config.ExportName) == "" {
		config.ExportName = "runTuiView"
	}
	if strings.TrimSpace(config.ModuleURL) == "" {
		return nil, errors.New("TuiView module URL is required")
	}
	parsed, err := url.Parse(config.ModuleURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse TuiView module URL")
	}
	if parsed.Scheme != "file" || !filepath.IsAbs(parsed.Path) {
		return nil, errors.New("TuiView module URL must be an absolute file URL")
	}
	if strings.TrimSpace(config.PluginID) == "" {
		return nil, errors.New("plugin ID is required")
	}
	if !filepath.IsAbs(config.DaemonSocketPath) {
		return nil, errors.New("daemon socket path must be absolute")
	}
	if strings.TrimSpace(config.StateStoreID) == "" {
		return nil, errors.New("Session state store ID is required")
	}
	if config.Stdin == nil {
		config.Stdin = os.Stdin
	}
	if config.Stdout == nil {
		config.Stdout = os.Stdout
	}
	if config.Stderr == nil {
		config.Stderr = os.Stderr
	}
	return &Host{config: config}, nil
}

// Run supervises the TuiView host until it exits or ctx is canceled.
func (h *Host) Run(ctx context.Context, onReady func()) error {
	readyReported := false
	reportReady := func() {
		if readyReported {
			return
		}
		readyReported = true
		if onReady != nil {
			onReady()
		}
	}
	var runErr error
	for attempt := uint(0); attempt <= h.config.RestartLimit; attempt++ {
		err := h.runAttempt(ctx, reportReady)
		restoreErr := restoreTerminal(h.config.Stdout)
		if ctx.Err() != nil {
			if err != nil && restoreErr != nil {
				return stderrors.Join(err, errors.Wrap(restoreErr, "restore terminal"))
			}
			if err != nil {
				return err
			}
			return restoreErr
		}
		if err == nil {
			return restoreErr
		}
		runErr = err
		if restoreErr != nil {
			runErr = stderrors.Join(runErr, errors.Wrap(restoreErr, "restore terminal"))
		}
	}
	return runErr
}

func (h *Host) runAttempt(ctx context.Context, onReady func()) (runErr error) {
	proxy, err := startUnixProxy(ctx, h.config.DaemonSocketPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := proxy.close(); err != nil {
			runErr = stderrors.Join(runErr, errors.Wrap(err, "close private Resource proxy"))
		}
	}()

	loader, err := materializeLoader()
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(filepath.Dir(loader)); err != nil {
			runErr = stderrors.Join(runErr, errors.Wrap(err, "remove TUI loader"))
		}
	}()

	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		return errors.Wrap(err, "create TUI readiness pipe")
	}
	defer readyRead.Close()

	// #nosec G204: BunPath selects the configured runtime; its arguments are fixed.
	cmd := exec.Command(h.config.BunPath, "run", loader)
	cmd.Stdin = h.config.Stdin
	cmd.Stdout = h.config.Stdout
	cmd.Stderr = h.config.Stderr
	cmd.ExtraFiles = []*os.File{readyWrite}
	cmd.Env = append(os.Environ(), h.environment(proxy.endpoint())...)
	if err := cmd.Start(); err != nil {
		readyWrite.Close()
		return errors.Wrap(err, "start Bun TuiView host")
	}
	readyWrite.Close()

	readyResult := make(chan error, 1)
	go scanReady(readyRead, readyResult)
	waitResult := make(chan error, 1)
	go func() { waitResult <- cmd.Wait() }()

	ready := false
	reportAttemptReady := func() {
		if ready {
			return
		}
		ready = true
		if onReady != nil {
			onReady()
		}
	}
	proxyDone := proxy.done
	for {
		select {
		case err := <-readyResult:
			readyResult = nil
			if err != nil {
				return h.stopChild(cmd, waitResult, err)
			}
			reportAttemptReady()
		case err := <-waitResult:
			if !ready && readyResult != nil {
				if readyErr := <-readyResult; readyErr == nil {
					reportAttemptReady()
				}
			}
			if err != nil {
				return errors.Wrap(err, "Bun TuiView host exited")
			}
			if !ready {
				return errors.New("Bun TuiView host exited before readiness")
			}
			return nil
		case <-ctx.Done():
			return h.stopChild(cmd, waitResult, nil)
		case err := <-proxyDone:
			if err != nil {
				return h.stopChild(cmd, waitResult, err)
			}
			proxyDone = nil
		}
	}
}

func (h *Host) environment(endpoint string) []string {
	return []string{
		"SPACEWAVE_TUI_MODULE_URL=" + h.config.ModuleURL,
		"SPACEWAVE_TUI_EXPORT=" + h.config.ExportName,
		"SPACEWAVE_TUI_READY_FD=3",
		"SPACEWAVE_TUI_ENDPOINT=" + endpoint,
		"SPACEWAVE_TUI_PLUGIN_ID=" + h.config.PluginID,
		"SPACEWAVE_TUI_SESSION_INDEX=" + strconv.FormatUint(uint64(h.config.SessionIndex), 10),
		"SPACEWAVE_TUI_SESSION_OBJECT_KEY=" + h.config.SessionObjectKey,
		"SPACEWAVE_TUI_SPACE_NAME=" + h.config.SpaceName,
		"SPACEWAVE_TUI_STATE_STORE_ID=" + h.config.StateStoreID,
	}
}

func (h *Host) stopChild(cmd *exec.Cmd, waitResult <-chan error, cause error) error {
	if cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	select {
	case waitErr := <-waitResult:
		if cause != nil {
			return cause
		}
		if waitErr != nil && !isSignalExit(waitErr) {
			return errors.Wrap(waitErr, "stop Bun TuiView host")
		}
		return nil
	case <-time.After(childExitDeadline):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-waitResult
		if cause != nil {
			return cause
		}
		return errors.New("Bun TuiView host did not stop after interrupt")
	}
}

func materializeLoader() (string, error) {
	data, err := loaderFS.ReadFile("host-loader.ts")
	if err != nil {
		return "", errors.Wrap(err, "read embedded TUI loader")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", errors.Wrap(err, "resolve user cache directory")
	}
	baseDir := filepath.Join(cacheDir, "spacewave", "tui")
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return "", errors.Wrap(err, "create TUI runtime root")
	}
	dir, err := os.MkdirTemp(baseDir, "loader-")
	if err != nil {
		return "", errors.Wrap(err, "create TUI loader directory")
	}
	path := filepath.Join(dir, "host-loader.ts")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", errors.Wrap(err, "write TUI loader")
	}
	return path, nil
}

func scanReady(reader io.Reader, result chan<- error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == readyMarker {
			result <- nil
			return
		}
	}
	if err := scanner.Err(); err != nil {
		result <- errors.Wrap(err, "read TUI readiness")
		return
	}
	result <- errors.New("TuiView host closed readiness pipe before ready")
}

func restoreTerminal(writer io.Writer) error {
	file, ok := writer.(*os.File)
	if !ok {
		return nil
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	_, err = io.WriteString(file, terminalRestore)
	return err
}

func isSignalExit(err error) bool {
	var exitErr *exec.ExitError
	if !stderrors.As(err, &exitErr) || exitErr.ProcessState == nil {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled()
}
