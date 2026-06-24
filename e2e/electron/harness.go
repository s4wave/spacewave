//go:build !skip_e2e && !js

// Package electron provides an opt-in Electron E2E harness backed by the
// existing Bldr desktop runtime.
package electron

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/s4wave/spacewave/bldr/devtool"
	"github.com/sirupsen/logrus"
)

const (
	defaultCDPReadyTimeout = 10 * time.Minute
	defaultCDPConnectRetry = 45 * time.Second
	cdpReadyTimeoutEnv     = "E2E_ELECTRON_CDP_READY_TIMEOUT"
	cdpShutdownTimeout     = 15 * time.Second
)

// Harness owns a Bldr desktop runtime plus a Playwright CDP attachment to the
// Electron renderer.
type Harness struct {
	ctx context.Context

	cancel context.CancelFunc

	repoRoot          string
	stateRoot         string
	artifactDir       string
	spacewaveDataRoot string
	cdpPort           int
	controlPort       int
	bldrSrc           string
	le                *logrus.Entry
	startSeq          int
	logFiles          []string

	done    chan struct{}
	doneErr error

	restoreEnv []func()

	pw      *playwright.Playwright
	browser playwright.Browser
}

// Boot starts Bldr's current desktop runtime and waits until Electron exposes
// its debug-only CDP endpoint.
func Boot(ctx context.Context, le *logrus.Entry) (_ *Harness, retErr error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}
	stateRoot := filepath.Join(repoRoot, ".bldr", "e2e-electron")
	if err := os.RemoveAll(stateRoot); err != nil {
		return nil, errors.Wrap(err, "clear electron e2e state root")
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, errors.Wrap(err, "create electron e2e state root")
	}
	artifactDir := filepath.Join(stateRoot, "artifacts", "runtime")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create electron e2e artifact dir")
	}
	spacewaveDataRoot := filepath.Join(stateRoot, "sw")
	if err := os.MkdirAll(spacewaveDataRoot, 0o755); err != nil {
		return nil, errors.Wrap(err, "create electron e2e spacewave data root")
	}

	port, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find CDP port")
	}
	controlPort, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find Electron e2e control port")
	}

	bldrSrcPath, err := filepath.Rel(filepath.Join(stateRoot, "src"), repoRoot)
	if err != nil {
		return nil, errors.Wrap(err, "resolve bldr source path")
	}

	hctx, cancel := context.WithCancel(ctx)
	h := &Harness{
		ctx:               ctx,
		cancel:            cancel,
		repoRoot:          repoRoot,
		stateRoot:         stateRoot,
		artifactDir:       artifactDir,
		spacewaveDataRoot: spacewaveDataRoot,
		cdpPort:           port,
		controlPort:       controlPort,
		bldrSrc:           bldrSrcPath,
		le:                le,
		done:              make(chan struct{}),
	}
	defer func() {
		if retErr != nil {
			h.Release()
		}
	}()

	h.restoreEnv = append(h.restoreEnv,
		setEnv("BLDR_ELECTRON_REMOTE_DEBUGGING_PORT", strconv.Itoa(port)),
		setEnv("BLDR_ELECTRON_E2E_CONTROL_PORT", strconv.Itoa(controlPort)),
		setEnv("BLDR_PLUGIN_STATE_PATH", filepath.Join(stateRoot, "electron-user-data")),
		setEnv("SPACEWAVE_DATA_DIR", spacewaveDataRoot),
	)

	if err := h.startDesktopRuntime(ctx, hctx, cancel); err != nil {
		return nil, err
	}

	return h, nil
}

// ConnectDriver attaches Playwright to Electron through the Chrome DevTools
// Protocol endpoint exposed by the Electron main process.
func (h *Harness) ConnectDriver() error {
	ctx, cancel := context.WithTimeout(h.ctx, defaultCDPConnectRetry)
	defer cancel()
	return h.connectDriver(ctx)
}

func (h *Harness) connectDriver(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		// Electron can answer /json/version before the CDP websocket is ready to
		// survive a full Playwright attach, especially immediately after relaunch.
		if err := h.connectDriverOnce(); err != nil {
			lastErr = err
			h.disconnectDriver()
			h.le.WithError(err).Debug("playwright CDP connect failed; retrying")
		} else {
			return nil
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return errors.Wrap(lastErr, "connect playwright over CDP before timeout")
			}
			return errors.Wrap(ctx.Err(), "connect playwright over CDP")
		case <-h.done:
			return h.desktopRuntimeErr("desktop runtime exited before playwright CDP driver connected")
		case <-ticker.C:
		}
	}
}

func (h *Harness) connectDriverOnce() error {
	pw, err := playwright.Run()
	if err != nil {
		return errors.Wrap(err, "start playwright")
	}
	h.pw = pw

	browser, err := pw.Chromium.ConnectOverCDP(h.CDPEndpoint())
	if err != nil {
		_ = pw.Stop()
		h.pw = nil
		return errors.Wrap(err, "connect playwright over CDP")
	}
	h.browser = browser
	return nil
}

// Relaunch terminates the current Electron runtime, starts it again with the
// same state root, and reconnects the Playwright CDP driver.
func (h *Harness) Relaunch(ctx context.Context) error {
	h.disconnectDriver()
	if err := h.stopDesktopRuntime(); err != nil {
		return err
	}

	hctx, cancel := context.WithCancel(h.ctx)
	if err := h.startDesktopRuntime(ctx, hctx, cancel); err != nil {
		return err
	}
	if err := h.connectDriver(ctx); err != nil {
		return err
	}
	return nil
}

// CDPEndpoint returns the local Electron CDP HTTP endpoint.
func (h *Harness) CDPEndpoint() string {
	return "http://127.0.0.1:" + strconv.Itoa(h.cdpPort)
}

// E2EControlEndpoint returns the local Electron-main e2e control endpoint.
func (h *Harness) E2EControlEndpoint() string {
	return "http://127.0.0.1:" + strconv.Itoa(h.controlPort)
}

// ControlEndpoint returns the local Electron main e2e control endpoint.
func (h *Harness) ControlEndpoint() string {
	return h.E2EControlEndpoint()
}

// SetDesktopState projects a desktop runtime fixture through the Electron-main
// e2e control surface.
func (h *Harness) SetDesktopState(ctx context.Context, state any) error {
	// encoding/json: the fixture is an arbitrary any projected over the e2e
	// control HTTP surface; no proto type or fastjson struct encoder fits.
	body, err := json.Marshal(state)
	if err != nil {
		return errors.Wrap(err, "marshal desktop runtime state")
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		h.ControlEndpoint()+"/desktop-state",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("set desktop state returned HTTP %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// ResetDesktopState clears desktop runtime and tray fixtures installed through
// the Electron-main e2e control surface.
func (h *Harness) ResetDesktopState(ctx context.Context) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		h.ControlEndpoint()+"/desktop-state",
		nil,
	)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("reset desktop state returned HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// StateRoot returns the isolated Bldr state root used by the harness.
func (h *Harness) StateRoot() string { return h.stateRoot }

// ArtifactDir returns the E2E artifact directory.
func (h *Harness) ArtifactDir() string { return h.artifactDir }

// SpacewaveDataRoot returns the short scratch SPACEWAVE_DATA_DIR.
func (h *Harness) SpacewaveDataRoot() string { return h.spacewaveDataRoot }

// LastLogFilePath returns the devtool log path for the latest runtime start.
func (h *Harness) LastLogFilePath() string {
	if len(h.logFiles) == 0 {
		return ""
	}
	return h.logFiles[len(h.logFiles)-1]
}

// RepoRoot returns the project repository root used by the harness.
func (h *Harness) RepoRoot() string { return h.repoRoot }

// CLISocketPath returns the dev-mode Spacewave daemon socket path.
func (h *Harness) CLISocketPath() string {
	return filepath.Join(h.repoRoot, ".spacewave", "spacewave.sock")
}

// WaitForPage waits until a renderer page is visible through the CDP driver.
func (h *Harness) WaitForPage(ctx context.Context) (playwright.Page, error) {
	if h.browser == nil {
		return nil, errors.New("playwright CDP browser is not connected")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, browserCtx := range h.browser.Contexts() {
			for _, page := range browserCtx.Pages() {
				if page.IsClosed() {
					continue
				}
				if strings.HasPrefix(page.URL(), "app://") {
					return page, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.done:
			return nil, h.desktopRuntimeErr("desktop runtime exited before renderer page appeared")
		case <-ticker.C:
		}
	}
}

// AppPages returns the renderer pages visible through the CDP driver.
func (h *Harness) AppPages() []playwright.Page {
	if h.browser == nil {
		return nil
	}
	var pages []playwright.Page
	for _, browserCtx := range h.browser.Contexts() {
		for _, page := range browserCtx.Pages() {
			if page.IsClosed() {
				continue
			}
			if strings.HasPrefix(page.URL(), "app://") {
				pages = append(pages, page)
			}
		}
	}
	return pages
}

// WaitForNoAppPages waits until no app renderer pages are visible.
func (h *Harness) WaitForNoAppPages(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(h.AppPages()) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.done:
			return h.desktopRuntimeErr("desktop runtime exited before renderer pages closed")
		case <-ticker.C:
		}
	}
}

// WaitForAppPages waits until at least count renderer pages are visible.
func (h *Harness) WaitForAppPages(
	ctx context.Context,
	count int,
) ([]playwright.Page, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		pages := h.AppPages()
		if len(pages) >= count {
			return pages, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.done:
			return nil, h.desktopRuntimeErr("desktop runtime exited before renderer pages appeared")
		case <-ticker.C:
		}
	}
}

// WaitForAppPageCount waits until exactly count renderer pages are visible.
func (h *Harness) WaitForAppPageCount(
	ctx context.Context,
	count int,
) ([]playwright.Page, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		pages := h.AppPages()
		if len(pages) == count {
			return pages, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.done:
			return nil, h.desktopRuntimeErr("desktop runtime exited before renderer page count matched")
		case <-ticker.C:
		}
	}
}

// ActivateApp triggers the Electron app activation path through the opt-in e2e
// control endpoint.
func (h *Harness) ActivateApp(ctx context.Context) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		h.ControlEndpoint()+"/activate",
		nil,
	)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("activate app returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Release stops Playwright, cancels the Bldr desktop runtime, and restores env.
func (h *Harness) Release() {
	h.disconnectDriver()
	_ = h.stopDesktopRuntime()
	for _, v := range slices.Backward(h.restoreEnv) {
		v()
	}
	h.restoreEnv = nil
}

func (h *Harness) startDesktopRuntime(
	ctx context.Context,
	hctx context.Context,
	cancel context.CancelFunc,
) error {
	h.cancel = cancel
	h.done = make(chan struct{})
	h.doneErr = nil

	args := devtool.NewDevtoolArgs()
	args.Logger = h.le
	args.LogLevel = "debug"
	args.StatePath = h.stateRoot
	args.Watch = false
	args.WebRenderer = "electron"
	args.BldrSrcPath = h.bldrSrc
	args.MinifyEntrypoint = false
	h.startSeq++
	logPath := filepath.Join(h.artifactDir, fmt.Sprintf("devtool-start-%02d.log", h.startSeq))
	if err := args.LogFiles.Set("level=DEBUG;path=" + logPath); err != nil {
		return err
	}
	h.logFiles = append(h.logFiles, logPath)

	go func() {
		h.doneErr = args.ExecuteNativeProject(hctx)
		args.CloseLogFiles()
		close(h.done)
	}()

	cdpReadyTimeout, err := resolveCDPReadyTimeout()
	if err != nil {
		return err
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, cdpReadyTimeout)
	defer waitCancel()
	if err := h.waitForCDP(waitCtx); err != nil {
		_ = h.stopDesktopRuntime()
		return err
	}
	return nil
}

func (h *Harness) stopDesktopRuntime() error {
	if h.done == nil {
		return nil
	}

	select {
	case <-h.done:
		err := h.doneErr
		h.done = nil
		h.cancel = nil
		if err != nil {
			return errors.Wrap(err, "desktop runtime exited before it was stopped")
		}
		return nil
	default:
	}

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	<-h.done
	h.done = nil
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cdpShutdownTimeout)
	defer shutdownCancel()
	if err := h.waitForCDPClosed(shutdownCtx); err != nil {
		return err
	}
	return nil
}

func (h *Harness) disconnectDriver() {
	if h.browser != nil {
		_ = h.browser.Close()
		h.browser = nil
	}
	if h.pw != nil {
		_ = h.pw.Stop()
		h.pw = nil
	}
}

func (h *Harness) waitForCDP(ctx context.Context) error {
	url := h.CDPEndpoint() + "/json/version"
	client := &http.Client{Timeout: 500 * time.Millisecond}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for Electron CDP endpoint")
		case <-h.done:
			return h.desktopRuntimeErr("desktop runtime exited before CDP endpoint became ready")
		case <-ticker.C:
		}
	}
}

// waitForCDPClosed prevents relaunch from treating a previous Electron process
// as the newly started runtime when both launches reuse the same debug port.
func (h *Harness) waitForCDPClosed(ctx context.Context) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(h.cdpPort))
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
		if err != nil {
			return nil
		}
		_ = conn.Close()
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for stale Electron CDP endpoint to close")
		case <-ticker.C:
		}
	}
}

func (h *Harness) desktopRuntimeErr(msg string) error {
	if h.doneErr != nil {
		return errors.Wrap(h.doneErr, msg)
	}
	return errors.New(msg)
}

func resolveCDPReadyTimeout() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(cdpReadyTimeoutEnv))
	if raw == "" {
		return defaultCDPReadyTimeout, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrapf(err, "unsupported %s value %q", cdpReadyTimeoutEnv, raw)
	}
	if timeout <= 0 {
		return 0, errors.Errorf("%s must be a positive duration, got %q", cdpReadyTimeoutEnv, raw)
	}
	return timeout, nil
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func setEnv(key, value string) func() {
	prev, ok := os.LookupEnv(key)
	_ = os.Setenv(key, value)
	return func() {
		if ok {
			_ = os.Setenv(key, prev)
			return
		}
		_ = os.Unsetenv(key)
	}
}
