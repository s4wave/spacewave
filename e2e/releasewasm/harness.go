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
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	e2eharness "github.com/s4wave/spacewave/e2e/harness"
	"github.com/s4wave/spacewave/e2e/releasewasm/artifact"
	"github.com/sirupsen/logrus"
)

const (
	// releaseDistRelPath locates the compiled browser release in the repository.
	releaseDistRelPath = ".bldr-dist/build/js/spacewave-browser/dist"
	// prerenderDistRelPath locates the generated static pages.
	prerenderDistRelPath = "app/prerender/dist"
	// releaseWasmDistDirEnv selects a prebuilt browser release directory.
	releaseWasmDistDirEnv = "E2E_RELEASE_WASM_DIST_DIR"
	// releaseWasmPrerenderDistEnv selects the matching prebuilt static pages.
	releaseWasmPrerenderDistEnv = "E2E_RELEASE_WASM_PRERENDER_DIST_DIR"
	// releaseAuthConfigPath serves the harness's local authentication endpoints.
	releaseAuthConfigPath = "/api/auth/config"
)

// browserReleaseDescriptor is parsed field-by-field from browser-release.json
// via fastjson in browserRelease; it is never marshaled or unmarshaled by
// encoding/json, so it carries no struct tags.
type browserReleaseDescriptor struct {
	// SchemaVersion identifies the browser release descriptor format.
	SchemaVersion int
	// GenerationID identifies the built release.
	GenerationID string
	// ShellAssets lists the browser shell's entrypoints.
	ShellAssets browserReleaseShellAssets
	// PrerenderedRoutes lists routes with generated HTML.
	PrerenderedRoutes []string
	// RequiredStaticAssets lists assets the release must serve.
	RequiredStaticAssets []string
}

// browserReleaseShellAssets names the shell's scripts, styles, and runtime.
type browserReleaseShellAssets struct {
	// Entrypoint starts the browser shell.
	Entrypoint string
	// ServiceWorker names the offline worker script.
	ServiceWorker string
	// SharedWorker names the shared runtime script.
	SharedWorker string
	// Wasm names the runtime binary when present in the release.
	Wasm string
	// CSS lists the shell's stylesheets.
	CSS []string
}

// harness holds the release server and browser for one suite run.
type harness struct {
	// artifactDir receives diagnostics from the suite.
	artifactDir string
	// distDirs identifies the release assets served by this run.
	distDirs releaseWasmDistDirs
	// baseURL is the local HTTP origin.
	baseURL string
	// browserName selects the Playwright browser engine.
	browserName string
	// repoRoot is the source checkout used to build the release.
	repoRoot string
	// server serves assets until release shuts it down.
	server *http.Server
	// pw is the Playwright driver stopped by release.
	pw *playwright.Playwright
	// browser is the browser process closed by release.
	browser playwright.Browser
	// chromiumPolicy selects the shared Chromium launch configuration.
	chromiumPolicy *e2eharness.ChromiumLaunchPolicy
}

// releaseWasmDistDirs pairs browser assets with their prerendered pages.
type releaseWasmDistDirs struct {
	// releaseDist contains the compiled browser assets.
	releaseDist string
	// prerender contains the generated static pages.
	prerender string
}

// boot builds the release and starts its local server and browser.
func boot(ctx context.Context, le *logrus.Entry) (_ *harness, retErr error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}

	// Own the listening socket before building or starting any browser consumer.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Wrap(err, "listen for release browser harness")
	}
	defer func() {
		if retErr != nil {
			listener.Close()
		}
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	baseURL := "http://127.0.0.1:" + port
	var distDirs releaseWasmDistDirs
	if os.Getenv(localCDNEnv) == "1" {
		distDirs, err = prepareLocalCDN(ctx, le, repoRoot, baseURL)
	} else {
		distDirs, err = prepareReleaseWasmDist(ctx, le, repoRoot)
	}
	if err != nil {
		return nil, err
	}
	artifactDir := filepath.Join(repoRoot, ".bldr", "e2e-releasewasm", "artifacts")
	browserName, err := releaseWasmBrowserName()
	if err != nil {
		return nil, err
	}
	chromiumPolicy, err := e2eharness.NewChromiumLaunchPolicy(le)
	if err != nil {
		return nil, err
	}
	h := &harness{
		artifactDir:    artifactDir,
		distDirs:       distDirs,
		baseURL:        baseURL,
		browserName:    browserName,
		repoRoot:       repoRoot,
		chromiumPolicy: chromiumPolicy,
	}
	defer func() {
		if retErr != nil {
			h.release(le)
		}
	}()

	handler := releaseHandler(distDirs.releaseDist, distDirs.prerender, baseURL)
	if os.Getenv(localCDNEnv) == "1" {
		handler = &localCDNHandler{next: handler}
	}
	h.server = &http.Server{
		Addr:              "127.0.0.1:" + port,
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		if err := h.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			le.WithError(err).Error("release wasm server exited")
		}
	}()

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

	launch := func(gpu bool) (playwright.Browser, error) {
		opts := playwright.BrowserTypeLaunchOptions{Headless: new(true)}
		if browserName == "chromium" {
			opts = e2eharness.ChromiumLaunchOptions(true, gpu)
		}
		if os.Getenv(localCDNEnv) == "1" {
			// The static server rejects proxy requests; all browser network access
			// stays local, including requests made by service and shared workers.
			opts.Proxy = &playwright.Proxy{Server: baseURL, Bypass: new("127.0.0.1,localhost")}
		}
		return browserType.Launch(opts)
	}
	var browser playwright.Browser
	if browserName == "chromium" {
		browser, err = e2eharness.LaunchChromium(ctx, h.chromiumPolicy, launch)
	} else {
		browser, err = launch(false)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "launch %s", browserName)
	}
	h.browser = browser

	return h, nil
}

// getBaseURL returns the local release origin.
func (h *harness) getBaseURL() string { return h.baseURL }

// persistentBrowserContextLaunchOptions maps the shared Chromium launch
// options onto a persistent-context launch. Non-Chromium browsers stay plain.
func persistentBrowserContextLaunchOptions(
	browserName string,
	gpu bool,
) playwright.BrowserTypeLaunchPersistentContextOptions {
	options := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: new(true),
	}
	if browserName == "chromium" {
		launchOptions := e2eharness.ChromiumLaunchOptions(true, gpu)
		options.Headless = launchOptions.Headless
		options.Channel = launchOptions.Channel
		options.Args = launchOptions.Args
	}
	return options
}

// prepareReleaseWasmDist resolves or builds assets matching the current inputs.
func prepareReleaseWasmDist(ctx context.Context, le *logrus.Entry, repoRoot string) (releaseWasmDistDirs, error) {
	identity, err := computeReleaseWasmArtifactIdentity(ctx, repoRoot)
	if err != nil {
		return releaseWasmDistDirs{}, errors.Wrap(err, "compute release artifact identity")
	}
	storeDir := releaseWasmArtifactStoreDir(repoRoot)

	prebuiltDirs, prebuilt, err := prebuiltReleaseWasmDistDirs(repoRoot)
	if err != nil {
		return releaseWasmDistDirs{}, err
	}
	requireFresh := false
	if prebuilt {
		validationErr := artifact.Validate(prebuiltDirs.releaseDist, prebuiltDirs.prerender, identity)
		if validationErr == nil {
			le.WithFields(logrus.Fields{
				"dist":      prebuiltDirs.releaseDist,
				"identity":  identity.Digest,
				"prerender": prebuiltDirs.prerender,
			}).Info("release artifact cache hit")
			return prebuiltDirs, nil
		}
		requireFresh = true
		le.WithError(validationErr).WithField("identity", identity.Digest).Info("prebuilt release artifact rejected; rebuilding")
	}

	resolved, err := e2eharness.Resolve(ctx, le, e2eharness.ResolveOptions{
		LockDir:      storeDir,
		LockName:     "build",
		RequireFresh: requireFresh,
	}, newReleaseShape(le, repoRoot, storeDir, identity))
	if err != nil {
		return releaseWasmDistDirs{}, err
	}
	return releaseWasmDistDirs{
		releaseDist: resolved.releaseDir,
		prerender:   resolved.prerenderDir,
	}, nil
}

// prebuiltReleaseWasmDistDirs resolves the optional pair of prebuilt directories.
func prebuiltReleaseWasmDistDirs(repoRoot string) (releaseWasmDistDirs, bool, error) {
	distDir := strings.TrimSpace(os.Getenv(releaseWasmDistDirEnv))
	prerenderDir := strings.TrimSpace(os.Getenv(releaseWasmPrerenderDistEnv))
	if distDir == "" && prerenderDir == "" {
		return releaseWasmDistDirs{}, false, nil
	}
	if distDir == "" || prerenderDir == "" {
		return releaseWasmDistDirs{}, true, errors.Errorf("%s and %s must be set together", releaseWasmDistDirEnv, releaseWasmPrerenderDistEnv)
	}
	return releaseWasmDistDirs{
		releaseDist: repoPath(repoRoot, distDir),
		prerender:   repoPath(repoRoot, prerenderDir),
	}, true, nil
}

// repoPath resolves a configured path relative to the repository when needed.
func repoPath(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

// releaseWasmBrowserName selects a supported browser from the environment.
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

// releaseWasmBuildScript returns the configured release build command.
func releaseWasmBuildScript() string {
	script := strings.TrimSpace(os.Getenv("E2E_RELEASE_WASM_BUILD_SCRIPT"))
	if script == "" {
		return "build:release:web:e2e"
	}
	return script
}

// buildReleaseWeb builds the release with the selected compiler or lazy fixture.
func buildReleaseWeb(ctx context.Context, repoRoot string) error {
	if os.Getenv("E2E_RELEASE_WASM_LAZY_PLUGIN_FIXTURE") == "1" {
		return runBun(
			ctx,
			repoRoot,
			"run",
			"bldr",
			"--",
			"--state-path=.bldr-dist",
			"--build-type=release",
			"build",
			"-b",
			"release-web-lazy-plugin-fixture",
		)
	}

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
		return runBun(ctx, repoRoot, "run", "build:release:web:e2e:tinygo")
	}
	return runBun(ctx, repoRoot, "run", releaseWasmBuildScript())
}

// playwrightBrowserType resolves the selected browser engine from the driver.
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

// quickstartSmokeArtifactPath names the current test's smoke diagnostics.
func (h *harness) quickstartSmokeArtifactPath(t testing.TB) string {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return filepath.Join(h.artifactDir, name+".json")
}

// quickstartRuntimeTraceArtifactPath names the current test's browser trace.
func (h *harness) quickstartRuntimeTraceArtifactPath(t testing.TB) string {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return filepath.Join(h.artifactDir, name+".chromium-trace.json")
}

// newPage creates an isolated page with diagnostics attached.
func (h *harness) newPage(t testing.TB) playwright.Page {
	t.Helper()
	page, _ := h.newPageWithDiagnosticsControl(t)
	return page
}

// newBrowserContext gives each test isolated storage. WebKit's ephemeral
// contexts reject OPFS access, so its tests use a temporary persistent profile.
func (h *harness) newBrowserContext(t testing.TB) playwright.BrowserContext {
	t.Helper()
	var ctx playwright.BrowserContext
	if h.browserName == "webkit" {
		ctx = h.newPersistentBrowserContext(t, t.TempDir())
	} else {
		var err error
		ctx, err = h.browser.NewContext(h.newContextOptions(t))
		if err != nil {
			t.Fatalf("new browser context: %v", err)
		}
	}
	t.Cleanup(func() {
		if err := ctx.Close(); err != nil {
			t.Logf("close browser context: %v", err)
		}
	})
	return ctx
}

// newPageWithDiagnosticsControl also returns a callback that mutes error capture.
func (h *harness) newPageWithDiagnosticsControl(t testing.TB) (playwright.Page, func()) {
	t.Helper()

	ctx := h.newBrowserContext(t)

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	muteDiagnostics := h.attachPageDiagnostics(t, page)
	return page, muteDiagnostics
}

// newDedicatedWorkerPage disables SharedWorker before any application script runs.
func (h *harness) newDedicatedWorkerPage(t testing.TB) playwright.Page {
	t.Helper()

	ctx := h.newBrowserContext(t)

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

// newPageInContext adds a page with diagnostics to an existing browser context.
func (h *harness) newPageInContext(t testing.TB, ctx playwright.BrowserContext) playwright.Page {
	t.Helper()

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page in context: %v", err)
	}
	h.attachPageDiagnostics(t, page)
	return page
}

// newPersistentBrowserContext opens the selected browser with a persistent profile.
func (h *harness) newPersistentBrowserContext(t testing.TB, userDataDir string) playwright.BrowserContext {
	t.Helper()

	browserType, err := playwrightBrowserType(h.pw, h.browserName)
	if err != nil {
		t.Fatalf("resolve persistent release browser type: %v", err)
	}
	launchPersistent := func(gpu bool) (playwright.BrowserContext, error) {
		options := persistentBrowserContextLaunchOptions(h.browserName, gpu)
		device := h.newContextOptions(t)
		options.Viewport = device.Viewport
		options.Screen = device.Screen
		options.UserAgent = device.UserAgent
		options.DeviceScaleFactor = device.DeviceScaleFactor
		options.IsMobile = device.IsMobile
		options.HasTouch = device.HasTouch
		if os.Getenv(localCDNEnv) == "1" {
			options.Proxy = &playwright.Proxy{Server: h.baseURL, Bypass: new("127.0.0.1,localhost")}
		}
		return browserType.LaunchPersistentContext(userDataDir, options)
	}
	var ctx playwright.BrowserContext
	switch h.browserName {
	case "chromium":
		if h.chromiumPolicy == nil {
			t.Fatal("launch persistent release browser context: missing chromium launch policy")
		}
		ctx, err = e2eharness.LaunchChromium(t.Context(), h.chromiumPolicy, launchPersistent)
	default:
		ctx, err = launchPersistent(false)
	}
	if err != nil {
		t.Fatalf("launch persistent release browser context: %v", err)
	}
	return ctx
}

// attachPageDiagnostics fails the test on captured browser errors unless muted.
func (h *harness) attachPageDiagnostics(t testing.TB, page playwright.Page) func() {
	t.Helper()

	var errs []string
	var errsMu sync.Mutex
	muted := false
	recordBrowserError := func(msg string) {
		errsMu.Lock()
		defer errsMu.Unlock()
		if muted {
			return
		}
		errs = append(errs, msg)
	}
	muteDiagnostics := func() {
		errsMu.Lock()
		muted = true
		errsMu.Unlock()
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
				if !ignoreBrowserError(msg.Text()) && !isExpectedReleaseWasmConsoleError(msg) {
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
			if !ignoreBrowserError(msg.Text()) && !isExpectedReleaseWasmConsoleError(msg) {
				t.Logf("browser console error location: %+v", msg.Location())
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
		if strings.HasPrefix(url, h.baseURL) &&
			!strings.HasSuffix(url, "/.vite/manifest.json") &&
			!isExpectedReleaseWasmHTTPError(url) {
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
	return muteDiagnostics
}

// browserPageErrorMessage preserves the browser stack when Playwright supplies it.
func browserPageErrorMessage(err error) string {
	var pwErr *playwright.Error
	if stderrors.As(err, &pwErr) && pwErr.Stack != "" {
		return pwErr.Stack
	}
	return err.Error()
}

// isRelevantReleaseWasmRequest identifies runtime and plugin asset requests.
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

// isExpectedReleaseWasmHTTPError identifies endpoints intentionally absent from
// the static release server. The app probes auth configuration even when the
// release proof runs without a cloud auth service.
func isExpectedReleaseWasmHTTPError(url string) bool {
	return strings.HasSuffix(url, "/api/auth/config") ||
		(os.Getenv(localCDNEnv) == "1" && url == cdn.DefaultBaseURL+"/"+cdn.ProvisionedSpaceID+"/root.packedmsg")
}

// isExpectedReleaseWasmConsoleError recognizes the browser's resource error for
// the auth probe when the static origin is unavailable during an offline test.
func isExpectedReleaseWasmConsoleError(msg playwright.ConsoleMessage) bool {
	location := msg.Location()
	return location != nil && isExpectedReleaseWasmHTTPError(location.URL) &&
		strings.HasPrefix(msg.Text(), "Failed to load resource:")
}

// isBrowserAbortedRequest recognizes cancellation during browser navigation.
func isBrowserAbortedRequest(failure string) bool {
	return failure == "cancelled" || strings.Contains(failure, "net::ERR_ABORTED")
}

// newContextOptions applies the optional Playwright device profile.
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
		return playwright.BrowserNewContextOptions{}
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

// ignoreBrowserError filters expected runtime diagnostics from browser errors.
func ignoreBrowserError(msg string) bool {
	return strings.Contains(msg, "cache disabled") ||
		strings.Contains(msg, "detected ctrl+shift+r") ||
		strings.Contains(msg, "web document is closed") ||
		strings.HasPrefix(msg, "level=debug ") ||
		strings.HasPrefix(msg, "level=info ")
}

// browserRelease fetches and decodes the release descriptor from the local server.
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
		SchemaVersion: v.GetInt("schemaVersion"),
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

// release closes the browser, driver, and HTTP server.
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

// releaseHandler serves release assets and prerendered pages from their roots.
func releaseHandler(distDir, staticDir, endpoint string) http.Handler {
	// Delegate filesystem path resolution to rooted file servers.
	fileServer := http.FileServer(http.Dir(distDir))
	staticServer := http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir)))
	authConfigHandler := releaseAuthConfigHandler(endpoint)

	// Apply release headers before dispatching auth, static, and bundle requests.
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		if req.URL.Path == releaseAuthConfigPath {
			authConfigHandler.ServeHTTP(rw, req)
			return
		}
		if strings.HasSuffix(req.URL.Path, ".wasm.gz") || strings.HasSuffix(req.URL.Path, ".mjs.gz") {
			rw.Header().Set("Content-Encoding", "gzip")
		}
		if strings.HasSuffix(req.URL.Path, ".wasm.gz") {
			rw.Header().Set("Content-Type", "application/wasm")
		}
		if strings.HasSuffix(req.URL.Path, ".mjs.gz") {
			rw.Header().Set("Content-Type", "application/javascript")
		}
		if strings.HasPrefix(req.URL.Path, "/static/") {
			staticServer.ServeHTTP(rw, req)
			return
		}
		if staticPath, ok := resolveStaticHTML(staticDir, req.URL.Path); ok {
			http.ServeFile(rw, req, staticPath)
			return
		}
		fileServer.ServeHTTP(rw, req)
	})
}

// releaseAuthConfigHandler describes the local authentication endpoints.
func releaseAuthConfigHandler(endpoint string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		resp := &api.AuthConfigResponse{
			SsoBaseUrl:       endpoint + "/api/auth/sso/start",
			ExchangeUrl:      endpoint + "/api/auth/sso/code/exchange",
			ConfirmUrl:       endpoint + "/api/auth/sso/confirm",
			AccountBaseUrl:   endpoint,
			PublicBaseUrl:    endpoint,
			GoogleSsoEnabled: false,
			GithubSsoEnabled: false,
			TurnstileSiteKey: "",
		}
		data, err := resp.MarshalVT()
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.WriteHeader(http.StatusOK)
		if _, err := rw.Write(data); err != nil {
			return
		}
	})
}

// resolveStaticHTML maps a route to an existing prerendered HTML page.
func resolveStaticHTML(staticDir, reqPath string) (string, bool) {
	clean := strings.Trim(strings.Split(reqPath, "?")[0], "/")
	if clean == "" {
		clean = "index"
	}
	if strings.Contains(clean, "..") {
		return "", false
	}
	// clean rejects ".." above; the join stays inside staticDir.
	path := filepath.Join(staticDir, clean+".html")
	// #nosec G703 -- traversal rejected above; path stays inside staticDir.
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

// runBun runs a build command in dir with output attached to the test process.
func runBun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
