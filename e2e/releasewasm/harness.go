//go:build !js

package releasewasm

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/sirupsen/logrus"
)

const (
	releaseDistRelPath   = ".bldr-dist/build/js/spacewave-dist/dist"
	prerenderDistRelPath = "app/prerender/dist"
)

type browserReleaseDescriptor struct {
	SchemaVersion        int                       `json:"schemaVersion"`
	GenerationID         string                    `json:"generationId"`
	ShellAssets          browserReleaseShellAssets `json:"shellAssets"`
	PrerenderedRoutes    []string                  `json:"prerenderedRoutes"`
	RequiredStaticAssets []string                  `json:"requiredStaticAssets"`
}

type browserReleaseShellAssets struct {
	Entrypoint    string   `json:"entrypoint"`
	ServiceWorker string   `json:"serviceWorker"`
	SharedWorker  string   `json:"sharedWorker"`
	Wasm          string   `json:"wasm"`
	CSS           []string `json:"css"`
}

type harness struct {
	artifactDir string
	baseURL     string
	browserName string
	server      *http.Server
	pw          *playwright.Playwright
	browser     playwright.Browser
}

func boot(ctx context.Context, le *logrus.Entry) (_ *harness, retErr error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}

	if err := os.RemoveAll(filepath.Join(repoRoot, prerenderDistRelPath)); err != nil {
		return nil, errors.Wrap(err, "clean prerender dist")
	}
	if err := os.RemoveAll(filepath.Join(repoRoot, ".bldr-dist")); err != nil {
		return nil, errors.Wrap(err, "clean release dist state")
	}

	le.Info("building release web bundle")
	if err := buildReleaseWeb(ctx, repoRoot); err != nil {
		return nil, errors.Wrap(err, "build release web bundle")
	}

	distDir := filepath.Join(repoRoot, releaseDistRelPath)
	le.Info("building prerender hydrate bundle")
	if err := runBun(ctx, repoRoot, "run", "vite", "build", "--config", "app/prerender/vite.hydrate.config.ts"); err != nil {
		return nil, errors.Wrap(err, "build prerender hydrate bundle")
	}
	le.Info("building prerender ssr bundle")
	if err := runBun(ctx, repoRoot, "run", "vite", "build", "--config", "app/prerender/vite.ssr.config.ts"); err != nil {
		return nil, errors.Wrap(err, "build prerender ssr bundle")
	}
	le.Info("running prerender build")
	if err := runBun(ctx, repoRoot, "./app/prerender/ssr-dist/build.js", "--dist-dir", distDir); err != nil {
		return nil, errors.Wrap(err, "run prerender build")
	}

	if _, err := os.Stat(filepath.Join(distDir, "browser-release.json")); err != nil {
		return nil, errors.Wrap(err, "stat browser-release.json")
	}
	staticDir := filepath.Join(repoRoot, prerenderDistRelPath)
	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err != nil {
		return nil, errors.Wrap(err, "stat prerender index.html")
	}

	port, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find free port")
	}
	baseURL := "http://127.0.0.1:" + port
	artifactDir := filepath.Join(repoRoot, ".bldr", "e2e-releasewasm", "artifacts")
	browserName, err := releaseWasmBrowserName()
	if err != nil {
		return nil, err
	}
	h := &harness{
		artifactDir: artifactDir,
		baseURL:     baseURL,
		browserName: browserName,
	}
	defer func() {
		if retErr != nil {
			h.release(le)
		}
	}()

	h.server = &http.Server{
		Addr:              "127.0.0.1:" + port,
		Handler:           releaseHandler(distDir, staticDir),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		if err := h.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			le.WithError(err).Error("release wasm server exited")
		}
	}()
	if err := h.waitForReady(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for release server")
	}

	le.WithField("browser", browserName).Info("installing playwright driver")
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{browserName},
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}); err != nil {
		return nil, errors.Wrap(err, "install playwright")
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, errors.Wrap(err, "start playwright")
	}
	h.pw = pw

	browserType, err := playwrightBrowserType(pw, browserName)
	if err != nil {
		return nil, err
	}

	launchOpts := playwright.BrowserTypeLaunchOptions{
		Headless: new(true),
	}
	if browserName == "chromium" {
		launchOpts.Args = []string{
			"--allow-loopback-in-peer-connection",
			"--disable-features=WebRtcHideLocalIpsWithMdns",
		}
	}
	browser, err := browserType.Launch(launchOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "launch %s", browserName)
	}
	h.browser = browser

	return h, nil
}

func (h *harness) getBaseURL() string { return h.baseURL }

func releaseWasmBrowserName() (string, error) {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("E2E_RELEASE_WASM_BROWSER")))
	switch name {
	case "", "chromium":
		return "chromium", nil
	case "firefox":
		return "firefox", nil
	case "webkit":
		return "webkit", nil
	default:
		return "", errors.Errorf("unsupported E2E_RELEASE_WASM_BROWSER %q", name)
	}
}

func releaseWasmBuildScript() string {
	script := strings.TrimSpace(os.Getenv("E2E_RELEASE_WASM_BUILD_SCRIPT"))
	if script == "" {
		return "build:release:web:e2e"
	}
	return script
}

func buildReleaseWeb(ctx context.Context, repoRoot string) error {
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		return err
	}
	if compiler == releaseWasmCompilerGoScript {
		return runBun(ctx, repoRoot, "run", "build:release:web:e2e:goscript")
	}
	if err := applyReleaseWasmTinyGoCompilerEnv(); err != nil {
		return errors.Wrap(err, "apply release wasm TinyGo compiler env")
	}
	if compiler == releaseWasmCompilerTinyGo {
		return runBun(ctx, repoRoot, "run", "bldr", "--", "--state-path=.bldr-dist", "--build-type=release", "build", "-b", "release-web-e2e-tinygo")
	}
	return runBun(ctx, repoRoot, "run", releaseWasmBuildScript())
}

func playwrightBrowserType(pw *playwright.Playwright, browserName string) (playwright.BrowserType, error) {
	switch browserName {
	case "chromium":
		return pw.Chromium, nil
	case "firefox":
		return pw.Firefox, nil
	case "webkit":
		return pw.WebKit, nil
	default:
		return nil, errors.Errorf("unsupported E2E_RELEASE_WASM_BROWSER %q", browserName)
	}
}

func (h *harness) quickstartSmokeArtifactPath(t testing.TB) string {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return filepath.Join(h.artifactDir, name+".json")
}

func (h *harness) quickstartRuntimeTraceArtifactPath(t testing.TB) string {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return filepath.Join(h.artifactDir, name+".chromium-trace.json")
}

func (h *harness) newPage(t testing.TB) playwright.Page {
	t.Helper()

	ctx, err := h.browser.NewContext(h.newContextOptions(t))
	if err != nil {
		t.Fatalf("new browser context: %v", err)
	}
	t.Cleanup(func() {
		if err := ctx.Close(); err != nil {
			t.Logf("close browser context: %v", err)
		}
	})

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	h.attachPageDiagnostics(t, page)
	return page
}

func (h *harness) newDedicatedWorkerPage(t testing.TB) playwright.Page {
	t.Helper()

	ctx, err := h.browser.NewContext(h.newContextOptions(t))
	if err != nil {
		t.Fatalf("new browser context: %v", err)
	}
	t.Cleanup(func() {
		if err := ctx.Close(); err != nil {
			t.Logf("close browser context: %v", err)
		}
	})

	script := `
Object.defineProperty(globalThis, 'SharedWorker', {
	configurable: true,
	value: undefined,
});
`
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install dedicated-worker init script: %v", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	h.attachPageDiagnostics(t, page)
	return page
}

func (h *harness) newPageInContext(t testing.TB, ctx playwright.BrowserContext) playwright.Page {
	t.Helper()

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page in context: %v", err)
	}
	h.attachPageDiagnostics(t, page)
	return page
}

func (h *harness) attachPageDiagnostics(t testing.TB, page playwright.Page) {
	t.Helper()

	var errs []string
	var errsMu sync.Mutex
	recordBrowserError := func(msg string) {
		errsMu.Lock()
		defer errsMu.Unlock()
		errs = append(errs, msg)
	}
	consoleTrace := os.Getenv("E2E_RELEASE_WASM_CONSOLE_TRACE") == "1"
	page.OnFrameNavigated(func(frame playwright.Frame) {
		if frame.ParentFrame() != nil {
			return
		}
		t.Logf("browser navigated: %s", frame.URL())
	})
	if os.Getenv("E2E_RELEASE_WASM_HTTP_TRACE") == "1" {
		page.OnRequest(func(req playwright.Request) {
			url := req.URL()
			if !isRelevantReleaseWasmRequest(url) {
				return
			}
			t.Logf("browser request: %s %s", req.Method(), url)
		})
		page.OnResponse(func(resp playwright.Response) {
			url := resp.URL()
			if !isRelevantReleaseWasmRequest(url) {
				return
			}
			t.Logf("browser response: %d %s", resp.Status(), url)
		})
	}
	page.OnRequestFailed(func(req playwright.Request) {
		url := req.URL()
		if !isRelevantReleaseWasmRequest(url) {
			return
		}
		failure := req.Failure().Error()
		msg := "browser request failed: " + req.Method() + " " + url + ": " + failure
		if isBrowserAbortedRequest(failure) {
			t.Log(msg)
			return
		}
		recordBrowserError(msg)
	})
	page.OnWorker(func(worker playwright.Worker) {
		if consoleTrace {
			t.Logf("browser worker: %s", worker.URL())
		}
		worker.OnConsole(func(msg playwright.ConsoleMessage) {
			switch msg.Type() {
			case "error":
				if !ignoreBrowserError(msg.Text()) {
					recordBrowserError("worker console error: " + msg.Text())
				}
			case "warning":
				if consoleTrace {
					t.Logf("browser worker warning: %s", msg.Text())
				}
			default:
				if consoleTrace {
					t.Logf("browser worker %s: %s", msg.Type(), msg.Text())
				}
			}
		})
		if consoleTrace {
			worker.OnClose(func(worker playwright.Worker) {
				t.Logf("browser worker closed: %s", worker.URL())
			})
		}
	})
	page.On("console", func(msg playwright.ConsoleMessage) {
		switch msg.Type() {
		case "error":
			if !ignoreBrowserError(msg.Text()) {
				recordBrowserError("console error: " + msg.Text())
			}
		case "warning":
			if consoleTrace {
				t.Logf("browser warning: %s", msg.Text())
			}
		default:
			if consoleTrace {
				t.Logf("browser %s: %s", msg.Type(), msg.Text())
			}
		}
	})
	page.On("pageerror", func(err error) {
		msg := browserPageErrorMessage(err)
		if !ignoreBrowserError(msg) {
			recordBrowserError("page error: " + msg)
		}
	})
	page.On("response", func(resp playwright.Response) {
		if resp.Status() < 400 {
			return
		}
		url := resp.URL()
		if strings.HasPrefix(url, h.baseURL) && !strings.HasSuffix(url, "/.vite/manifest.json") {
			recordBrowserError("http " + resp.StatusText() + ": " + resp.URL())
			return
		}
		t.Logf("browser http warning: %d %s", resp.Status(), url)
	})
	t.Cleanup(func() {
		errsMu.Lock()
		defer errsMu.Unlock()
		if len(errs) != 0 {
			t.Fatalf("browser errors: %v", errs)
		}
	})
}

func browserPageErrorMessage(err error) string {
	var pwErr *playwright.Error
	if stderrors.As(err, &pwErr) && pwErr.Stack != "" {
		return pwErr.Stack
	}
	return err.Error()
}

func isRelevantReleaseWasmRequest(url string) bool {
	if strings.Contains(url, "runtime.wasm") ||
		strings.Contains(url, "runtime-wasm") ||
		strings.Contains(url, "/shw") ||
		strings.Contains(url, "/sw-") ||
		strings.Contains(url, "/b/pd/") ||
		strings.Contains(url, "/b/pa/") ||
		strings.Contains(url, "/b/pkg/") ||
		strings.Contains(url, "/quickstart/drive") ||
		strings.Contains(url, "/entrypoint/") {
		return true
	}
	return false
}

func isBrowserAbortedRequest(failure string) bool {
	return strings.Contains(failure, "net::ERR_ABORTED")
}

func (h *harness) newContextOptions(t testing.TB) playwright.BrowserNewContextOptions {
	t.Helper()

	deviceName := strings.TrimSpace(os.Getenv("PLAYWRIGHT_BROWSER_DEVICE"))
	if deviceName == "" {
		return playwright.BrowserNewContextOptions{}
	}

	device := h.pw.Devices[deviceName]
	if device == nil {
		names := make([]string, 0, len(h.pw.Devices))
		for name := range h.pw.Devices {
			names = append(names, name)
		}
		slices.Sort(names)
		t.Fatalf("unknown PLAYWRIGHT_BROWSER_DEVICE %q. Available devices: %s", deviceName, strings.Join(names, ", "))
	}

	return playwright.BrowserNewContextOptions{
		Viewport:          device.Viewport,
		Screen:            device.Screen,
		UserAgent:         new(device.UserAgent),
		DeviceScaleFactor: new(device.DeviceScaleFactor),
		IsMobile:          new(device.IsMobile),
		HasTouch:          new(device.HasTouch),
	}
}

func ignoreBrowserError(msg string) bool {
	return strings.Contains(msg, "cache disabled") ||
		strings.Contains(msg, "detected ctrl+shift+r") ||
		strings.Contains(msg, "web document is closed") ||
		strings.HasPrefix(msg, "level=debug ") ||
		strings.HasPrefix(msg, "level=info ")
}

func (h *harness) browserRelease(ctx context.Context) (*browserReleaseDescriptor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/browser-release.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("browser-release.json returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	desc := &browserReleaseDescriptor{
		SchemaVersion: int(v.GetInt("schemaVersion")),
		GenerationID:  string(v.GetStringBytes("generationId")),
		ShellAssets: browserReleaseShellAssets{
			Entrypoint:    string(v.GetStringBytes("shellAssets", "entrypoint")),
			ServiceWorker: string(v.GetStringBytes("shellAssets", "serviceWorker")),
			SharedWorker:  string(v.GetStringBytes("shellAssets", "sharedWorker")),
			Wasm:          string(v.GetStringBytes("shellAssets", "wasm")),
		},
	}
	for _, css := range v.GetArray("shellAssets", "css") {
		desc.ShellAssets.CSS = append(desc.ShellAssets.CSS, string(css.GetStringBytes()))
	}
	for _, route := range v.GetArray("prerenderedRoutes") {
		desc.PrerenderedRoutes = append(desc.PrerenderedRoutes, string(route.GetStringBytes()))
	}
	for _, asset := range v.GetArray("requiredStaticAssets") {
		desc.RequiredStaticAssets = append(desc.RequiredStaticAssets, string(asset.GetStringBytes()))
	}
	return desc, nil
}

func (h *harness) release(le *logrus.Entry) {
	if h.browser != nil {
		if err := h.browser.Close(); err != nil {
			le.WithError(err).Warn("close browser")
		}
	}
	if h.pw != nil {
		if err := h.pw.Stop(); err != nil {
			le.WithError(err).Warn("stop playwright")
		}
	}
	if h.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.server.Shutdown(ctx); err != nil {
			le.WithError(err).Warn("shutdown release server")
		}
	}
}

func releaseHandler(distDir, staticDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(distDir))
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		if strings.HasSuffix(req.URL.Path, ".wasm.gz") || strings.HasSuffix(req.URL.Path, ".mjs.gz") {
			rw.Header().Set("Content-Encoding", "gzip")
		}
		if strings.HasSuffix(req.URL.Path, ".wasm.gz") {
			rw.Header().Set("Content-Type", "application/wasm")
		}
		if strings.HasSuffix(req.URL.Path, ".mjs.gz") {
			rw.Header().Set("Content-Type", "application/javascript")
		}
		if after, ok := strings.CutPrefix(req.URL.Path, "/static/"); ok {
			http.ServeFile(rw, req, filepath.Join(staticDir, after))
			return
		}
		if staticPath, ok := resolveStaticHTML(staticDir, req.URL.Path); ok {
			http.ServeFile(rw, req, staticPath)
			return
		}
		fileServer.ServeHTTP(rw, req)
	})
}

func resolveStaticHTML(staticDir, reqPath string) (string, bool) {
	clean := strings.Trim(strings.Split(reqPath, "?")[0], "/")
	if clean == "" {
		clean = "index"
	}
	if strings.Contains(clean, "..") {
		return "", false
	}
	path := filepath.Join(staticDir, clean+".html")
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

func runBun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (h *harness) waitForReady(ctx context.Context) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/browser-release.json", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func findFreePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}
