//go:build !skip_e2e && !js

// Package electron provides an opt-in Electron E2E harness backed by the
// existing Bldr desktop runtime.
package electron

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/s4wave/spacewave/bldr/devtool"
	"github.com/sirupsen/logrus"
)

const cdpReadyTimeout = 10 * time.Minute

// Harness owns a Bldr desktop runtime plus a Playwright CDP attachment to the
// Electron renderer.
type Harness struct {
	cancel context.CancelFunc

	stateRoot string
	cdpPort   int

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

	port, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find CDP port")
	}

	bldrSrcPath, err := filepath.Rel(filepath.Join(stateRoot, "src"), repoRoot)
	if err != nil {
		return nil, errors.Wrap(err, "resolve bldr source path")
	}

	hctx, cancel := context.WithCancel(ctx)
	h := &Harness{
		cancel:    cancel,
		stateRoot: stateRoot,
		cdpPort:   port,
		done:      make(chan struct{}),
	}
	defer func() {
		if retErr != nil {
			h.Release()
		}
	}()

	h.restoreEnv = append(h.restoreEnv,
		setEnv("BLDR_ELECTRON_REMOTE_DEBUGGING_PORT", strconv.Itoa(port)),
		setEnv("BLDR_PLUGIN_STATE_PATH", filepath.Join(stateRoot, "electron-user-data")),
	)

	args := devtool.NewDevtoolArgs()
	args.Logger = le
	args.LogLevel = "debug"
	args.StatePath = stateRoot
	args.Watch = false
	args.WebRenderer = "electron"
	args.BldrSrcPath = bldrSrcPath
	args.MinifyEntrypoint = false

	go func() {
		h.doneErr = args.ExecuteNativeProject(hctx)
		args.CloseLogFiles()
		close(h.done)
	}()

	waitCtx, waitCancel := context.WithTimeout(ctx, cdpReadyTimeout)
	defer waitCancel()
	if err := h.waitForCDP(waitCtx); err != nil {
		return nil, err
	}

	return h, nil
}

// ConnectDriver attaches Playwright to Electron through the Chrome DevTools
// Protocol endpoint exposed by the Electron main process.
func (h *Harness) ConnectDriver() error {
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

// CDPEndpoint returns the local Electron CDP HTTP endpoint.
func (h *Harness) CDPEndpoint() string {
	return "http://127.0.0.1:" + strconv.Itoa(h.cdpPort)
}

// StateRoot returns the isolated Bldr state root used by the harness.
func (h *Harness) StateRoot() string { return h.stateRoot }

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
			if strings.HasPrefix(page.URL(), "app://") {
				pages = append(pages, page)
			}
		}
	}
	return pages
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

// Release stops Playwright, cancels the Bldr desktop runtime, and restores env.
func (h *Harness) Release() {
	if h.browser != nil {
		_ = h.browser.Close()
		h.browser = nil
	}
	if h.pw != nil {
		_ = h.pw.Stop()
		h.pw = nil
	}
	if h.cancel != nil {
		h.cancel()
	}
	if h.done != nil {
		<-h.done
		h.done = nil
	}
	for i := len(h.restoreEnv) - 1; i >= 0; i-- {
		h.restoreEnv[i]()
	}
	h.restoreEnv = nil
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

func (h *Harness) desktopRuntimeErr(msg string) error {
	if h.doneErr != nil {
		return errors.Wrap(h.doneErr, msg)
	}
	return errors.New(msg)
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
