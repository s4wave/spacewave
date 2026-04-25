//go:build !js

// Package wasm provides a Go test harness that boots the real bldr
// start:web:wasm lifecycle and exposes the running app for e2e testing.
package wasm

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccall"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/s4wave/spacewave/bldr/devtool"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

// Harness boots and manages the bldr start:web:wasm lifecycle for e2e testing.
// One harness is intended per test package. The harness boots the devtool
// bus, compiles plugins, and starts the HTTP server once. Individual tests
// create isolated browser sessions via NewSession.
type Harness struct {
	devtool       *devtool.DevtoolBus
	projConfig    *bldr_project.ProjectConfig
	projRef       directive.Reference
	manifestRefs  []directive.Reference
	manifestWaits []manifestWait
	port          int
	baseURL       string
	headless      bool
	ctx           context.Context
	cancel        context.CancelFunc
	wasmErr       error
	wasmDone      chan struct{}

	// Browser process (populated by LaunchBrowser, shared across sessions).
	pw      *playwright.Playwright
	browser playwright.Browser

	// Compiled TypeScript test scripts (populated by CompileScripts).
	scripts CompiledScripts

	// PeerWatcher tracks browser peers across sessions (lazy init).
	peerWatcher     *PeerWatcher
	peerWatcherOnce sync.Once
	peerLeaseMu     sync.Mutex
	peerLeases      map[string]*TestSession
	pageSessionMu   sync.Mutex
	pageSessions    map[playwright.Page]*TestSession
}

// Boot starts the full wasm app lifecycle: builds the devtool bus, syncs
// dist sources, loads and optionally mutates the project config, starts the
// resolveHeadless determines whether the browser should run headless.
// If explicitly set via WithHeadless, that value wins. Otherwise,
// headless is the default unless E2E_WASM_HEADLESS=false or
// E2E_WASM_HEADLESS=0.
func resolveHeadless(explicit *bool) bool {
	if explicit != nil {
		return *explicit
	}
	v := strings.ToLower(os.Getenv("E2E_WASM_HEADLESS"))
	return v != "false" && v != "0"
}

// project controller (which compiles plugin manifests), builds the web
// entrypoint and runtime.wasm, and serves the app over HTTP.
//
// The returned Harness must be released with Release when done.
func Boot(ctx context.Context, le *logrus.Entry, opts ...Option) (_ *Harness, retErr error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	repoRoot := o.repoRoot
	if repoRoot == "" {
		var err error
		repoRoot, err = gitroot.FindRepoRoot()
		if err != nil {
			return nil, errors.Wrap(err, "find repo root")
		}
	}

	stateRoot, err := buildHarnessStateRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	if err := clearHarnessStateRoot(stateRoot); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, errors.Wrap(err, "create state root")
	}

	hctx, cancel := context.WithCancel(ctx)

	h := &Harness{
		ctx:      hctx,
		cancel:   cancel,
		headless: resolveHeadless(o.headless),
	}
	defer func() {
		if retErr != nil {
			h.Release()
		}
	}()

	d, err := devtool.BuildDevtoolBus(hctx, le, repoRoot, stateRoot, false)
	if err != nil {
		return nil, errors.Wrap(err, "build devtool bus")
	}
	h.devtool = d

	bldrVersion, bldrSum, bldrSrcPath, err := resolveBldrDependency(repoRoot)
	if err != nil {
		return nil, err
	}

	if err := d.SyncDistSources(bldrVersion, bldrSum, bldrSrcPath); err != nil {
		return nil, errors.Wrap(err, "sync dist sources")
	}

	projConfig, err := loadProjectConfig(repoRoot)
	if err != nil {
		return nil, err
	}

	// Wire the devtool remote so plugin manifests resolve against the testbed.
	if projConfig.Remotes == nil {
		projConfig.Remotes = make(map[string]*bldr_project.RemoteConfig)
	}
	projConfig.Remotes["devtool"] = &bldr_project.RemoteConfig{
		EngineId:       d.GetWorldEngineID(),
		PeerId:         d.GetVolume().GetPeerID().String(),
		ObjectKey:      d.GetPluginHostObjectKey(),
		LinkObjectKeys: []string{d.GetPluginHostObjectKey()},
	}

	for _, mut := range o.configMutators {
		if err := mut(projConfig); err != nil {
			return nil, errors.Wrap(err, "apply config mutator")
		}
	}

	if err := projConfig.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate project config")
	}
	h.projConfig = projConfig

	// Start the project controller which builds plugin manifests.
	projCtrlConf := bldr_project_controller.NewConfig(
		repoRoot,
		stateRoot,
		projConfig,
		false, // watch
		true,  // fetchManifests
	)
	projCtrlConf.FetchManifestRemote = "devtool"
	_, _, projRef, err := loader.WaitExecControllerRunning(
		hctx,
		d.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start project controller")
	}
	h.projRef = projRef

	// Resolve startup values from the loaded config.
	appID := projConfig.GetId()
	startPlugins := projConfig.GetStart().GetPlugins()
	webStartupSrcPath, _ := projConfig.GetStart().ParseWebStartupPath()

	port, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find free port")
	}
	h.port = port
	addr := "127.0.0.1:" + strconv.Itoa(port)
	h.baseURL = "http://" + addr

	// Run the wasm lifecycle in the background; it blocks on ListenAndServe.
	h.wasmDone = make(chan struct{})
	go func() {
		h.wasmErr = d.ExecuteWebWasm(
			hctx,
			repoRoot,
			false, // minifyEntrypoint
			true,  // devMode
			addr,
			appID,
			startPlugins,
			webStartupSrcPath,
			true, // useDedicatedWorkers (Playwright can capture dedicated worker console)
		)
		close(h.wasmDone)
	}()

	if err := h.waitForReady(hctx); err != nil {
		return nil, errors.Wrap(err, "wait for wasm readiness")
	}
	if err := h.writeBrowserReleaseDescriptor(); err != nil {
		return nil, errors.Wrap(err, "write browser release descriptor")
	}

	if err := h.assertStartupManifestFetches(); err != nil {
		return nil, errors.Wrap(err, "assert startup manifest fetches")
	}

	// Wait for the asserted startup plugin manifests to be built before
	// returning. TestMain launches Playwright only after Boot returns.
	if err := h.waitForManifests(hctx); err != nil {
		return nil, errors.Wrap(err, "wait for manifest builds")
	}

	return h, nil
}

// CompileScripts discovers and compiles *.ts files in the given directory
// into ESM modules served at /e2e/*.mjs. The compiled modules externalize
// shared web packages (react, @aptre/bldr, etc.) so the browser resolves
// them via the app's import map, sharing module instances with the running app.
func (h *Harness) CompileScripts(dir string) error {
	outDir := filepath.Join(h.devtool.GetStateRoot(), "entry", "web", "wasm", "e2e")
	scripts, err := CompileTestScripts(dir, outDir)
	if err != nil {
		return err
	}
	h.scripts = scripts
	return nil
}

// ScriptOutDir returns the output directory for compiled test scripts.
// Files written here are served at /e2e/*.mjs by the devtool HTTP server.
func (h *Harness) ScriptOutDir() string {
	return filepath.Join(h.devtool.GetStateRoot(), "entry", "web", "wasm", "e2e")
}

// SetScripts sets the compiled test script map, for use by downstream repos
// that compile scripts with a custom resolver via CompileTestScriptsFor.
func (h *Harness) SetScripts(scripts CompiledScripts) { h.scripts = scripts }

// Scripts returns the compiled test scripts. Returns nil if CompileScripts
// has not been called.
func (h *Harness) Scripts() CompiledScripts { return h.scripts }

func (h *Harness) writeBrowserReleaseDescriptor() error {
	entryDir := filepath.Join(h.devtool.GetStateRoot(), "entry", "web", "wasm")
	assets := []string{
		"/entrypoint/entrypoint.mjs",
		"/entrypoint/runtime.wasm",
		"/sw.mjs",
		"/shw.mjs",
	}
	for _, asset := range assets {
		path := filepath.Join(entryDir, strings.TrimPrefix(asset, "/"))
		if _, err := os.Stat(path); err != nil {
			return errors.Wrap(err, "stat "+asset)
		}
	}

	const descriptor = `{
  "schemaVersion": 1,
  "generationId": "e2e-dev",
  "shellAssets": {
    "entrypoint": "/entrypoint/entrypoint.mjs",
    "serviceWorker": "/sw.mjs",
    "sharedWorker": "/shw.mjs",
    "wasm": "/entrypoint/runtime.wasm",
    "css": []
  },
  "prerenderedRoutes": [],
  "requiredStaticAssets": []
}
`
	return os.WriteFile(filepath.Join(entryDir, "browser-release.json"), []byte(descriptor), 0o644)
}

// Script returns a JS expression that dynamically imports the named test
// script and calls its default export with the provided args. The expression
// is compatible with Playwright's Page.Evaluate(expr, args).
//
// Panics if the script is not found, which immediately surfaces missing
// scripts in tests.
func (h *Harness) Script(name string) string {
	url, ok := h.scripts[name]
	if !ok {
		panic("compiled script not found: " + name)
	}
	return "async (args) => (await import('" + url + "')).default(args)"
}

// Context returns the harness lifecycle context.
func (h *Harness) Context() context.Context { return h.ctx }

// BaseURL returns the HTTP base URL of the running app (e.g. http://127.0.0.1:12345).
func (h *Harness) BaseURL() string { return h.baseURL }

// Port returns the TCP port the HTTP server is listening on.
func (h *Harness) Port() int { return h.port }

// GetDevtoolBus returns the underlying DevtoolBus for advanced access.
func (h *Harness) GetDevtoolBus() *devtool.DevtoolBus { return h.devtool }

// GetProjectConfig returns the resolved project config.
func (h *Harness) GetProjectConfig() *bldr_project.ProjectConfig { return h.projConfig }

// Cleanup registers Release as a test cleanup function so the harness is
// torn down when the test or subtest finishes.
func (h *Harness) Cleanup(t testing.TB) { t.Cleanup(h.Release) }

func (h *Harness) leaseBrowserPeer(s *TestSession, p peer.ID) bool {
	key := string(p)
	h.peerLeaseMu.Lock()
	defer h.peerLeaseMu.Unlock()

	if h.peerLeases == nil {
		h.peerLeases = make(map[string]*TestSession)
	}

	owner := h.peerLeases[key]
	if owner != nil && owner != s {
		return false
	}
	h.peerLeases[key] = s
	return true
}

func (h *Harness) releaseBrowserPeerLease(s *TestSession, p peer.ID) {
	if len(p) == 0 {
		return
	}
	key := string(p)
	h.peerLeaseMu.Lock()
	defer h.peerLeaseMu.Unlock()

	if h.peerLeases[key] == s {
		delete(h.peerLeases, key)
	}
}

// Release tears down the harness: closes the shared browser process,
// cancels the context, waits for the HTTP server goroutine to exit,
// and releases all controllers and the devtool bus. Individual test
// sessions are released via their own cleanup (t.Cleanup).
func (h *Harness) Release() {
	h.closeBrowser()
	if h.peerWatcher != nil {
		h.peerWatcher.Release()
	}
	h.peerLeaseMu.Lock()
	h.peerLeases = nil
	h.peerLeaseMu.Unlock()
	if h.cancel != nil {
		h.cancel()
	}
	if h.wasmDone != nil {
		<-h.wasmDone
	}
	if h.projRef != nil {
		h.projRef.Release()
	}
	for _, ref := range h.manifestRefs {
		ref.Release()
	}
	h.manifestRefs = nil
	h.manifestWaits = nil
	if h.devtool != nil {
		h.devtool.Release()
	}
}

type manifestFetchRequest struct {
	pluginID    string
	buildTypes  []bldr_manifest.BuildType
	platformIDs []string
}

type manifestWait struct {
	req  manifestFetchRequest
	done <-chan error
}

const manifestBuildTimeout = 2 * time.Minute

func (r manifestFetchRequest) directive() directive.Directive {
	return bldr_manifest.NewFetchManifest(r.pluginID, r.buildTypes, r.platformIDs, 0)
}

func (r manifestFetchRequest) logFields() logrus.Fields {
	fields := logrus.Fields{
		"plugin":   r.pluginID,
		"platform": strings.Join(r.platformIDs, ","),
	}
	if len(r.buildTypes) != 0 {
		fields["build-type"] = r.buildTypes[0]
	}
	return fields
}

func (r manifestFetchRequest) summary() string {
	if len(r.buildTypes) == 0 {
		return r.pluginID + "[" + strings.Join(r.platformIDs, ",") + "]"
	}
	return r.pluginID + "/" + string(r.buildTypes[0]) + "[" + strings.Join(r.platformIDs, ",") + "]"
}

// loadProjectConfig reads and merges bldr.yaml and bldr.star at the repo root.
// bldr.star takes precedence over bldr.yaml when both exist.
func loadProjectConfig(repoRoot string) (*bldr_project.ProjectConfig, error) {
	yamlPath := filepath.Join(repoRoot, "bldr.yaml")
	starPath := filepath.Join(repoRoot, "bldr.star")

	yamlData, yamlErr := os.ReadFile(yamlPath)
	_, starErr := os.Stat(starPath)
	if yamlErr != nil && starErr != nil {
		return nil, errors.Wrap(yamlErr, "read bldr.yaml")
	}

	conf := &bldr_project.ProjectConfig{}

	// Load bldr.yaml as base config if it exists.
	if yamlErr == nil {
		if err := bldr_project.UnmarshalProjectConfig(yamlData, conf); err != nil {
			return nil, errors.Wrap(err, "unmarshal bldr.yaml")
		}
	}

	// Evaluate bldr.star and merge on top if it exists.
	if starErr == nil {
		result, err := bldr_project_starlark.Evaluate(starPath)
		if err != nil {
			return nil, errors.Wrap(err, "evaluate bldr.star")
		}
		if err := bldr_project.MergeProjectConfigs(conf, result.Config); err != nil {
			return nil, errors.Wrap(err, "merge bldr.star config")
		}
	}

	return conf, nil
}

// findFreePort allocates an ephemeral TCP port and returns it.
func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func (h *Harness) assertStartupManifestFetches() error {
	le := logrus.WithField("component", "harness")
	b := h.devtool.GetBus()
	for _, req := range h.startupManifestRequests() {
		if _, ok := h.projConfig.GetManifests()[req.pluginID]; !ok {
			return errors.Errorf("startup manifest %q not found in project config", req.pluginID)
		}

		done := make(chan error, 1)
		var signalOnce sync.Once
		signal := func(err error) {
			signalOnce.Do(func() {
				done <- err
				close(done)
			})
		}

		handler := directive.NewTypedCallbackHandler(
			func(v directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
				if len(v.GetValue().GetManifestRefs()) == 0 {
					signal(errors.Errorf("manifest %s resolved with no refs", req.pluginID))
					return
				}
				signal(nil)
			},
			nil,
			func() {
				signal(errors.Errorf("manifest %s disposed before value", req.pluginID))
			},
			nil,
		)

		le.WithFields(req.logFields()).Info("asserting manifest fetch")
		_, ref, err := b.AddDirective(req.directive(), handler)
		if err != nil {
			return errors.Wrapf(err, "assert manifest fetch for %s", req.pluginID)
		}
		h.manifestRefs = append(h.manifestRefs, ref)
		h.manifestWaits = append(h.manifestWaits, manifestWait{req: req, done: done})
	}
	return nil
}

// waitForManifests waits for all asserted startup plugin FetchManifest
// directives to resolve on the devtool bus. This ensures builds are complete
// before Playwright loads the app.
func (h *Harness) waitForManifests(ctx context.Context) error {
	le := logrus.WithField("component", "harness")
	waitCtx, cancel := context.WithTimeout(ctx, manifestBuildTimeout)
	defer cancel()

	fns := make([]ccall.CallConcurrentlyFunc, 0, len(h.manifestWaits))
	for _, wait := range h.manifestWaits {
		fns = append(fns, func(ctx context.Context) error {
			le.WithFields(wait.req.logFields()).Info("waiting for manifest build")
			select {
			case err := <-wait.done:
				if err != nil {
					return err
				}
				le.WithFields(wait.req.logFields()).Info("manifest build ready")
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	if err := ccall.CallConcurrently(waitCtx, fns...); err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			return errors.Errorf(
				"timed out after %s waiting for startup manifest callbacks: %s",
				manifestBuildTimeout,
				h.startupManifestSummary(),
			)
		}
		return errors.Wrap(err, "wait for startup manifest callbacks")
	}

	le.Info("all plugin manifests built")
	return nil
}

func (h *Harness) startupManifestSummary() string {
	parts := make([]string, 0, len(h.manifestWaits))
	for _, wait := range h.manifestWaits {
		parts = append(parts, wait.req.summary())
	}
	return strings.Join(parts, ", ")
}

func (h *Harness) startupManifestRequests() []manifestFetchRequest {
	return []manifestFetchRequest{
		{pluginID: "spacewave-core", platformIDs: []string{"web/js/wasm"}},
		{pluginID: "spacewave-debug", platformIDs: []string{"web/js/wasm"}},
		{pluginID: "web", platformIDs: []string{"web/js/wasm"}},
		{pluginID: "spacewave-web", platformIDs: []string{"js"}},
		{pluginID: "spacewave-app", platformIDs: []string{"js"}},
	}
}

// waitForReady polls the /bldr-dev/web-wasm/info endpoint until the server
// responds with 200 OK. Server readiness checks in test setup are the accepted
// exception to the no-polling rule.
func (h *Harness) waitForReady(ctx context.Context) error {
	infoURL := h.baseURL + "/bldr-dev/web-wasm/info"
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
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
		case <-h.wasmDone:
			if h.wasmErr != nil {
				return errors.Wrap(h.wasmErr, "wasm lifecycle failed during startup")
			}
			return errors.New("wasm lifecycle exited before server became ready")
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// bldrModPath is the Go module path for bldr.
const bldrModPath = "github.com/s4wave/spacewave/bldr"

// resolveBldrDependency determines the bldr module version/checksum and any
// local replace path. The source path is passed into SyncDistSources so the
// vendored dist source tree follows local bldr checkouts instead of re-vendoring
// an older module version.
func resolveBldrDependency(repoRoot string) (version, sum, srcPath string, err error) {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return resolveBldrDependencyFromGoMod(repoRoot)
	}
	for _, dep := range buildInfo.Deps {
		if dep.Path == bldrModPath {
			if dep.Replace != nil {
				if p, ok := resolveLocalModulePath(repoRoot, dep.Replace.Path); ok {
					srcPath = p
				}
				if dep.Replace.Version != "" && dep.Replace.Version != "(devel)" {
					return dep.Replace.Version, dep.Replace.Sum, srcPath, nil
				}
				if dep.Version != "" && dep.Version != "(devel)" {
					return dep.Version, dep.Sum, srcPath, nil
				}
				if srcPath != "" {
					return "", "", srcPath, nil
				}
				continue
			}
			if dep.Version != "" && dep.Version != "(devel)" {
				return dep.Version, dep.Sum, "", nil
			}
		}
	}
	return resolveBldrDependencyFromGoMod(repoRoot)
}

// resolveBldrDependencyFromGoMod falls back to repoRoot/go.mod when build info
// does not expose the dependency graph, which can happen in test binaries.
func resolveBldrDependencyFromGoMod(repoRoot string) (version, sum, srcPath string, err error) {
	if repoRoot == "" {
		return "", "", "", errors.New("unable to resolve bldr dependency")
	}
	goModPath := filepath.Join(repoRoot, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return "", "", "", errors.Wrap(err, "read go.mod")
	}
	mod, err := modfile.Parse(goModPath, goModData, nil)
	if err != nil {
		return "", "", "", errors.Wrap(err, "parse go.mod")
	}
	for _, repl := range mod.Replace {
		if repl.Old.Path != bldrModPath {
			continue
		}
		if p, ok := resolveLocalModulePath(repoRoot, repl.New.Path); ok {
			srcPath = p
			break
		}
	}
	for _, req := range mod.Require {
		if req.Mod.Path != bldrModPath {
			continue
		}
		if req.Mod.Version == "" || req.Mod.Version == "(devel)" {
			break
		}
		return req.Mod.Version, "", srcPath, nil
	}
	if srcPath != "" {
		return "", "", srcPath, nil
	}
	return "", "", "", errors.New("unable to resolve bldr dependency")
}

// resolveLocalModulePath resolves a local replace target relative to repoRoot.
func resolveLocalModulePath(repoRoot, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if filepath.IsAbs(path) {
		return path, true
	}
	if strings.HasPrefix(path, ".") {
		if repoRoot == "" {
			return "", false
		}
		return filepath.Clean(filepath.Join(repoRoot, path)), true
	}
	return "", false
}

// buildHarnessStateRoot returns a per-package harness state root.
//
// Recursive `go test ./e2e/wasm/...` runs boot multiple test binaries in
// parallel. They must not share the same `.bldr/e2e-wasm` directory or one
// package can delete `src/` while another is syncing it.
func buildHarnessStateRoot(repoRoot string) (string, error) {
	stateRoot := filepath.Join(repoRoot, ".bldr", "e2e-wasm")
	scope := "default"
	label := "wasm"
	cwd, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "get working directory")
	}
	rel, err := filepath.Rel(repoRoot, cwd)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		scope = rel
		label = filepath.Base(cwd)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", errors.Wrap(err, "get executable path")
	}
	sum := sha1.Sum([]byte(scope + "|" + filepath.Base(exe)))
	token := hex.EncodeToString(sum[:4])
	return filepath.Join(stateRoot, label+"-"+token), nil
}

var harnessStateCleanupGlobs = []string{
	"devtool.db*",
	"devtool.s4wave*",
	"logs",
	"src",
	"plugin",
	"build",
	"cli",
}

// clearHarnessStateRoot removes the transient .bldr entries that the harness
// needs to rebuild from a clean state. This matches the repo clean target more
// closely than deleting the entire .bldr tree.
func clearHarnessStateRoot(stateRoot string) error {
	for _, pattern := range harnessStateCleanupGlobs {
		matches, err := filepath.Glob(filepath.Join(stateRoot, pattern))
		if err != nil {
			return errors.Wrapf(err, "expand state cleanup pattern %q", pattern)
		}
		for _, path := range matches {
			if err := os.RemoveAll(path); err != nil {
				return errors.Wrapf(err, "remove state path %s", path)
			}
		}
	}
	return nil
}
