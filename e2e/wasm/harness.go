//go:build !js

// Package wasm provides a Go test harness that boots the real bldr
// start:web:wasm lifecycle and exposes the running app for e2e testing.
package wasm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
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
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/devtool"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	bldr_statepath "github.com/s4wave/spacewave/bldr/statepath"
	e2eharness "github.com/s4wave/spacewave/e2e/harness"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/sys/unix"
)

// Harness boots and manages the bldr start:web:wasm lifecycle for e2e testing.
// One harness is intended per test package. The harness boots the devtool
// bus, compiles plugins, and starts the HTTP server once. Individual tests
// choose NewClean* or NewRetainedState* helpers at the call site.
type Harness struct {
	devtool        *devtool.DevtoolBus
	projConfig     *bldr_project.ProjectConfig
	projRef        directive.Reference
	manifestRefs   []directive.Reference
	manifestWaits  []manifestWait
	le             *logrus.Entry
	chromiumPolicy *e2eharness.ChromiumLaunchPolicy
	port           int
	baseURL        string
	headless       bool
	browserName    string
	workerMode     WorkerMode
	manifestWait   time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
	wasmErr        error
	wasmDone       chan struct{}

	// Browser process (populated by LaunchBrowser, shared across sessions).
	pw      *playwright.Playwright
	browser playwright.Browser

	// Retained-state BrowserContext (lazy init). This is intentionally not used
	// by NewCleanSession/NewCleanBlankSession, which keep strict isolated
	// context semantics.
	retainedStateCtxMu sync.Mutex
	retainedStateCtx   playwright.BrowserContext

	// Compiled TypeScript test scripts (populated by CompileScripts).
	scripts CompiledScripts

	// PeerWatcher tracks browser peers across sessions (lazy init).
	peerWatcher     *PeerWatcher
	peerWatcherOnce sync.Once
	peerLeaseMu     sync.Mutex
	peerLeaseWaitCh chan struct{}
	peerLeases      map[string]*TestSession
	pageSessionMu   sync.Mutex
	pageSessions    map[playwright.Page]*TestSession

	retainedStateResourcePeerMu sync.Mutex
	retainedStateResourcePeer   peer.ID

	cloudEndpointClose func()

	stateRoot                 string
	preserveStartupBuildCache bool
	stateRootOwner            harnessStateRootOwner
	stateRootLock             *os.File
}

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

func resolveBrowserName(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("E2E_WASM_BROWSER"))); v != "" {
		return v
	}
	return "chromium"
}

// Boot starts the full wasm app lifecycle: builds the devtool bus, syncs
// dist sources, loads and optionally mutates the project config, starts the
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

	envStartupBuildCache, err := ResolveE2EWasmStartupBuildCacheEnabled()
	if err != nil {
		return nil, err
	}
	preserveStartupBuildCache := resolveStartupBuildCache(envStartupBuildCache, o.preserveStartupBuildCache)
	stateRoot, err := buildHarnessStateRoot(repoRoot, preserveStartupBuildCache)
	if err != nil {
		return nil, err
	}
	stableStateRoot, err := buildHarnessStateRoot(repoRoot, true)
	if err != nil {
		return nil, err
	}
	stateRootOwner, err := newHarnessStateRootOwner()
	if err != nil {
		return nil, err
	}
	reapHarnessCacheOffStateRoots(le, filepath.Dir(stateRoot), stateRoot, stableStateRoot, stateRootOwner)

	workerMode, err := ResolveE2EWasmWorkerMode(o.workerMode)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, errors.Wrap(err, "create state root")
	}
	stateRootLock, acquired, err := acquireHarnessStateRootLock(stateRoot)
	if err != nil {
		return nil, err
	}
	if !acquired {
		if !preserveStartupBuildCache {
			return nil, errors.Errorf("e2e wasm state root is already owned: %s", stateRoot)
		}
		sharedStateRoot := stateRoot
		stateRoot, err = buildHarnessStateRoot(repoRoot, false)
		if err != nil {
			return nil, err
		}
		preserveStartupBuildCache = false
		if err := os.MkdirAll(stateRoot, 0o755); err != nil {
			return nil, errors.Wrap(err, "create isolated state root")
		}
		stateRootLock, acquired, err = acquireHarnessStateRootLock(stateRoot)
		if err != nil {
			return nil, err
		}
		if !acquired {
			return nil, errors.Errorf("isolated e2e wasm state root is already owned: %s", stateRoot)
		}
		le.WithFields(logrus.Fields{
			"state-root":          sharedStateRoot,
			"isolated-state-root": stateRoot,
		}).Info("isolating concurrent e2e wasm startup build cache")
	}
	if preserveStartupBuildCache {
		le.WithField("state-root", stateRoot).Info("preserving e2e wasm startup build cache")
	}

	chromiumPolicy, err := e2eharness.NewChromiumLaunchPolicy(le)
	if err != nil {
		return nil, err
	}

	hctx, cancel := context.WithCancel(ctx)
	manifestWait := o.manifestBuildTimeout
	if manifestWait == 0 {
		manifestWait = defaultManifestBuildTimeout
	}

	h := &Harness{
		ctx:                       hctx,
		chromiumPolicy:            chromiumPolicy,
		cancel:                    cancel,
		headless:                  resolveHeadless(o.headless),
		browserName:               resolveBrowserName(o.browserName),
		workerMode:                workerMode,
		manifestWait:              manifestWait,
		le:                        le,
		stateRoot:                 stateRoot,
		preserveStartupBuildCache: preserveStartupBuildCache,
		stateRootOwner:            stateRootOwner,
		stateRootLock:             stateRootLock,
	}
	defer func() {
		if retErr != nil {
			h.Release()
		}
	}()
	if !preserveStartupBuildCache {
		if err := writeHarnessStateRootOwner(stateRoot, stateRootOwner); err != nil {
			return nil, err
		}
	}
	if err := bldr_statepath.ClearBuildState(stateRoot, preserveStartupBuildCache); err != nil {
		return nil, err
	}

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

	cloudEndpoint, stopCloudEndpoint, err := startE2ECloudAuthConfigEndpoint(stableE2ECloudAuthConfigAddr(stateRoot))
	if err != nil {
		return nil, errors.Wrap(err, "start cloud auth config endpoint")
	}
	h.cloudEndpointClose = stopCloudEndpoint

	projConfig, err := loadProjectConfig(repoRoot)
	if err != nil {
		return nil, err
	}
	if err := applyE2ECloudAuthConfigEndpoint(projConfig, cloudEndpoint); err != nil {
		return nil, errors.Wrap(err, "apply cloud auth config endpoint")
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
	startupManifestPreflights := devtool.ProjectOwnedStartupManifestPreflights(projConfig, "web/js/wasm")
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
			startupManifestPreflights,
			webStartupSrcPath,
			workerMode == WorkerModeDedicated,
		)
		close(h.wasmDone)
	}()

	if err := h.waitForReady(hctx); err != nil {
		return nil, errors.Wrap(err, "wait for wasm readiness")
	}
	if err := h.writeBrowserReleaseDescriptor(); err != nil {
		return nil, errors.Wrap(err, "write browser release descriptor")
	}

	if err := h.settleStartupManifests(hctx); err != nil {
		return nil, errors.Wrap(err, "settle startup manifests")
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

func (h *Harness) waitBrowserPeerLease(ctx context.Context, s *TestSession, p peer.ID) error {
	key := string(p)
	for {
		h.peerLeaseMu.Lock()
		if h.peerLeases == nil {
			h.peerLeases = make(map[string]*TestSession)
		}
		if h.peerLeaseWaitCh == nil {
			h.peerLeaseWaitCh = make(chan struct{})
		}

		owner := h.peerLeases[key]
		if owner == nil || owner == s {
			h.peerLeases[key] = s
			h.peerLeaseMu.Unlock()
			return nil
		}
		waitCh := h.peerLeaseWaitCh
		h.peerLeaseMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
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
		if h.peerLeaseWaitCh != nil {
			close(h.peerLeaseWaitCh)
			h.peerLeaseWaitCh = make(chan struct{})
		}
	}
}

func (h *Harness) browserPeerLeaseOwner(p peer.ID) *TestSession {
	if len(p) == 0 {
		return nil
	}
	key := string(p)
	h.peerLeaseMu.Lock()
	defer h.peerLeaseMu.Unlock()
	return h.peerLeases[key]
}

func (h *Harness) browserPeerLeaseCount() int {
	h.peerLeaseMu.Lock()
	defer h.peerLeaseMu.Unlock()
	return len(h.peerLeases)
}

func (h *Harness) getRetainedStateResourcePeer() peer.ID {
	h.retainedStateResourcePeerMu.Lock()
	defer h.retainedStateResourcePeerMu.Unlock()
	return h.retainedStateResourcePeer
}

func (h *Harness) setRetainedStateResourcePeer(p peer.ID) {
	h.retainedStateResourcePeerMu.Lock()
	defer h.retainedStateResourcePeerMu.Unlock()
	h.retainedStateResourcePeer = p
}

// Release tears down the harness: closes the shared browser process,
// cancels the context, waits for the HTTP server goroutine to exit,
// and releases all controllers and the devtool bus. Individual test
// sessions are released via their own cleanup (t.Cleanup).
func (h *Harness) Release() {
	h.closeRetainedStateContext()
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
	if h.cloudEndpointClose != nil {
		h.cloudEndpointClose()
		h.cloudEndpointClose = nil
	}
	if h.projRef != nil {
		h.projRef.Release()
	}
	h.releaseManifestFetches()
	if h.devtool != nil {
		h.devtool.Release()
	}
	if h.stateRootLock != nil {
		if err := h.stateRootLock.Close(); err != nil && h.le != nil {
			h.le.WithError(err).WithField("state-root", h.stateRoot).Error("release e2e wasm state root lock")
		}
		h.stateRootLock = nil
	}
	if h.stateRoot != "" && !h.preserveStartupBuildCache {
		if err := os.RemoveAll(h.stateRoot); err != nil && h.le != nil {
			h.le.WithError(err).WithField("state-root", h.stateRoot).Error("remove e2e wasm state root")
		}
	}
}

type manifestFetchRequest struct {
	pluginID    string
	buildTypes  []bldr_manifest.BuildType
	platformIDs []string
}

type manifestWait struct {
	req   manifestFetchRequest
	state *manifestWaitState
}

// Cold browser startup compiles Go wasm plugins and frontend Vite bundles
// concurrently. Keep the gate bounded, but long enough that slow cold builds
// fail on the actual phase instead of canceling startup manifest publication.
const defaultManifestBuildTimeout = 5 * time.Minute

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
	le := h.le.WithField("component", "harness")
	b := h.devtool.GetBus()
	for _, req := range h.startupManifestRequests() {
		if _, ok := h.projConfig.GetManifests()[req.pluginID]; !ok {
			return errors.Errorf("startup manifest %q not found in project config", req.pluginID)
		}

		waitState := newManifestWaitState(req.pluginID)

		le.WithFields(req.logFields()).Info("asserting manifest fetch")
		di, ref, err := b.AddDirective(req.directive(), waitState.handler())
		if err != nil {
			return errors.Wrapf(err, "assert manifest fetch for %s", req.pluginID)
		}
		di.AddIdleCallback(waitState.handleIdle)
		h.manifestRefs = append(h.manifestRefs, ref)
		h.manifestWaits = append(h.manifestWaits, manifestWait{req: req, state: waitState})
	}
	return nil
}

// settleStartupManifests re-runs the startup manifest preflight until the web
// build reaches a fixpoint: two consecutive passes produce identical manifest
// digests for every plugin. A slow host builds startup manifests in waves (a
// slow TinyGo core build invalidates dependent web/app manifests after an
// earlier pass), so a fixed pass count can return while a later invalidation
// wave is still pending and would hot-rebuild the app after the browser has
// loaded it, opening a serving outage mid-test. Settling to a fixpoint drains
// those waves before the harness serves the app.
func (h *Harness) settleStartupManifests(ctx context.Context) error {
	le := h.le.WithField("component", "harness")
	const maxPasses = 8
	var prev map[string]string
	for pass := 1; pass <= maxPasses; pass++ {
		digests, err := h.preflightStartupManifests(ctx)
		if err != nil {
			return err
		}
		if maps.Equal(prev, digests) {
			le.WithField("passes", pass).Info("startup manifests settled to build fixpoint")
			return nil
		}
		le.WithField("pass", pass).Info("startup manifests not yet stable, re-settling")
		prev = digests
	}
	return errors.Errorf(
		"startup manifests did not reach a build fixpoint after %d passes: %s",
		maxPasses,
		h.startupManifestSummary(),
	)
}

// preflightStartupManifests runs one settle pass: release prior fetches, assert
// the startup manifest fetches, wait for their builds, and return the settled
// per-plugin manifest digest so the caller can detect a build fixpoint.
func (h *Harness) preflightStartupManifests(ctx context.Context) (map[string]string, error) {
	h.releaseManifestFetches()
	if err := h.assertStartupManifestFetches(); err != nil {
		return nil, errors.Wrap(err, "assert startup manifest fetches")
	}
	if err := h.waitForManifests(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for manifest builds")
	}
	digests := make(map[string]string, len(h.manifestWaits))
	for _, wait := range h.manifestWaits {
		digests[wait.req.pluginID] = wait.state.digest()
	}
	return digests, nil
}

// SettleProjectManifest builds one lazy project plugin to the same fixpoint as
// browser startup manifests before a test path requests it from PluginHost.
func (h *Harness) SettleProjectManifest(pluginID string) error {
	preflight, ok := devtool.ProjectOwnedStartupManifestPreflight(h.projConfig, pluginID, "web/js/wasm")
	if !ok {
		return errors.Errorf("project manifest %q not found", pluginID)
	}
	req := manifestFetchRequest{
		pluginID:    preflight.PluginID,
		platformIDs: preflight.PlatformIDs,
	}

	ctx, cancel := context.WithTimeout(h.ctx, h.manifestWait*8)
	defer cancel()

	const maxPasses = 8
	var prev string
	for pass := 1; pass <= maxPasses; pass++ {
		digest, err := h.preflightManifestFetch(ctx, req)
		if err != nil {
			return err
		}
		if prev == digest {
			h.le.WithFields(req.logFields()).WithField("passes", pass).Info("lazy manifest settled to build fixpoint")
			return nil
		}
		h.le.WithFields(req.logFields()).WithField("pass", pass).Info("lazy manifest not yet stable, re-settling")
		prev = digest
	}
	return errors.Errorf("lazy manifest %s did not reach a build fixpoint after %d passes", req.summary(), maxPasses)
}

func (h *Harness) preflightManifestFetch(ctx context.Context, req manifestFetchRequest) (string, error) {
	if _, ok := h.projConfig.GetManifests()[req.pluginID]; !ok {
		return "", errors.Errorf("manifest %q not found in project config", req.pluginID)
	}

	waitState := newManifestWaitState(req.pluginID)
	h.le.WithFields(req.logFields()).Info("asserting lazy manifest fetch")
	di, ref, err := h.devtool.GetBus().AddDirective(req.directive(), waitState.handler())
	if err != nil {
		return "", errors.Wrapf(err, "assert manifest fetch for %s", req.pluginID)
	}
	defer ref.Release()
	di.AddIdleCallback(waitState.handleIdle)

	wait := manifestWait{req: req, state: waitState}
	if err := h.waitForManifest(ctx, wait); err != nil {
		return "", err
	}
	return waitState.digest(), nil
}

func (h *Harness) waitForManifest(ctx context.Context, wait manifestWait) error {
	waitCtx, cancel := context.WithTimeout(ctx, h.manifestWait)
	defer cancel()

	h.le.WithFields(wait.req.logFields()).Info("waiting for manifest build")
	select {
	case err := <-wait.state.done:
		if err != nil {
			return err
		}
		h.le.WithFields(wait.req.logFields()).Info("manifest build ready")
		return nil
	case <-waitCtx.Done():
		return errors.Errorf(
			"timed out after %s waiting for manifest callback: %s",
			h.manifestWait,
			wait.req.summary(),
		)
	}
}

func (h *Harness) releaseManifestFetches() {
	for _, ref := range h.manifestRefs {
		ref.Release()
	}
	h.manifestRefs = nil
	h.manifestWaits = nil
}

// waitForManifests waits for all asserted startup plugin FetchManifest
// directives to resolve on the devtool bus. This ensures builds are complete
// before Playwright loads the app.
func (h *Harness) waitForManifests(ctx context.Context) error {
	le := h.le.WithField("component", "harness")
	waitCtx, cancel := context.WithTimeout(ctx, h.manifestWait)
	defer cancel()

	fns := make([]ccall.CallConcurrentlyFunc, 0, len(h.manifestWaits))
	for _, wait := range h.manifestWaits {
		fns = append(fns, func(ctx context.Context) error {
			le.WithFields(wait.req.logFields()).Info("waiting for manifest build")
			select {
			case err := <-wait.state.done:
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
				h.manifestWait,
				h.startupManifestSummary(),
			)
		}
		return errors.Wrap(err, "wait for startup manifest callbacks")
	}

	le.Info("all plugin manifests built")
	return nil
}

type manifestWaitState struct {
	pluginID string
	done     chan error
	values   map[uint32]*bldr_manifest.FetchManifestValue
	idle     bool
	errs     []error
	signaled bool
	mtx      sync.Mutex
}

func newManifestWaitState(pluginID string) *manifestWaitState {
	return &manifestWaitState{
		pluginID: pluginID,
		done:     make(chan error, 1),
		values:   make(map[uint32]*bldr_manifest.FetchManifestValue),
	}
}

func (s *manifestWaitState) handler() directive.ReferenceHandler {
	return directive.NewTypedCallbackHandler(
		s.handleValueAdded,
		s.handleValueRemoved,
		func() {
			s.signal(errors.Errorf("manifest %s disposed before build settled", s.pluginID))
		},
		nil,
	)
}

func (s *manifestWaitState) handleValueAdded(v directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.values[v.GetValueID()] = v.GetValue()
	s.checkLocked()
}

func (s *manifestWaitState) handleValueRemoved(v directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.values, v.GetValueID())
}

func (s *manifestWaitState) handleIdle(isIdle bool, errs []error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.idle = isIdle
	s.errs = errs
	s.checkLocked()
}

func (s *manifestWaitState) checkLocked() {
	if s.signaled || !s.idle {
		return
	}
	for _, err := range s.errs {
		if err != nil {
			s.signalLocked(err)
			return
		}
	}
	for _, val := range s.values {
		if len(val.GetManifestRefs()) == 0 {
			continue
		}
		s.signalLocked(nil)
		return
	}
}

func (s *manifestWaitState) signal(err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.signalLocked(err)
}

func (s *manifestWaitState) signalLocked(err error) {
	if s.signaled {
		return
	}
	s.signaled = true
	s.done <- err
	close(s.done)
}

// digest returns a stable content digest over the settled manifest root refs.
// Two preflight passes that yield the same digest prove the plugin's web build
// has quiesced; a changed digest means a delayed invalidation wave rebuilt it.
func (s *manifestWaitState) digest() string {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	refs := make([]string, 0, len(s.values))
	for _, val := range s.values {
		for _, mref := range val.GetManifestRefs() {
			rootRef := mref.GetManifestRef().GetRootRef()
			if rootRef == nil {
				continue
			}
			b, err := rootRef.MarshalVT()
			if err != nil {
				continue
			}
			refs = append(refs, hex.EncodeToString(b))
		}
	}
	slices.Sort(refs)
	sum := sha256.Sum256([]byte(strings.Join(refs, ",")))
	return hex.EncodeToString(sum[:])
}

func (h *Harness) startupManifestSummary() string {
	parts := make([]string, 0, len(h.manifestWaits))
	for _, wait := range h.manifestWaits {
		parts = append(parts, wait.req.summary())
	}
	return strings.Join(parts, ", ")
}

func (h *Harness) startupManifestRequests() []manifestFetchRequest {
	preflights := devtool.ProjectOwnedStartupManifestPreflights(h.projConfig, "web/js/wasm")
	requests := make([]manifestFetchRequest, 0, len(preflights))
	for _, preflight := range preflights {
		requests = append(requests, manifestFetchRequest{
			pluginID:    preflight.PluginID,
			platformIDs: preflight.PlatformIDs,
		})
	}
	return requests
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

// bldrModPath is the Go module path that owns the bldr source tree.
const bldrModPath = "github.com/s4wave/spacewave"

// resolveBldrDependency determines the bldr module version/checksum and any
// local replace path. The source path is passed into SyncDistSources so the
// vendored dist source tree follows local bldr checkouts instead of re-vendoring
// an older module version.
func resolveBldrDependency(repoRoot string) (version, sum, srcPath string, err error) {
	repoModulePath, repoModuleErr := repoGoModulePath(repoRoot)
	if repoModuleErr == nil && repoModulePath != "" && repoModulePath != bldrModPath {
		if p, ok := resolveLocalModulePath("", repoRoot); ok {
			return "", "", p, nil
		}
		return "", "", repoRoot, nil
	}
	if repoModulePath == bldrModPath {
		if p, ok := resolveLocalModulePath("", repoRoot); ok {
			return "", "", p, nil
		}
		return "", "", repoRoot, nil
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return resolveBldrDependencyFromGoMod(repoRoot)
	}
	if buildInfo.Main.Path == bldrModPath {
		if p, ok := resolveLocalModulePath("", repoRoot); ok {
			return "", "", p, nil
		}
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
	if mod.Module != nil && mod.Module.Mod.Path == bldrModPath {
		return "", "", repoRoot, nil
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

func repoGoModulePath(repoRoot string) (string, error) {
	if repoRoot == "" {
		return "", errors.New("repo root is required")
	}
	goModPath := filepath.Join(repoRoot, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return "", errors.Wrap(err, "read go.mod")
	}
	mod, err := modfile.Parse(goModPath, goModData, nil)
	if err != nil {
		return "", errors.Wrap(err, "parse go.mod")
	}
	if mod.Module == nil {
		return "", errors.New("go.mod has no module path")
	}
	return mod.Module.Mod.Path, nil
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

const (
	harnessStateRootLockName           = ".e2e-lock"
	harnessStateRootOwnerName          = ".e2e-owner"
	harnessMarkerlessStateRootMaxAge   = 24 * time.Hour
	harnessStateRootTokenBytes         = 16
	harnessStateRootTokenEncodedLength = harnessStateRootTokenBytes * 2
)

type harnessStateRootOwner struct {
	pid             int
	createdUnixNano int64
	token           string
}

func acquireHarnessStateRootLock(stateRoot string) (*os.File, bool, error) {
	lock, err := os.OpenFile(filepath.Join(stateRoot, harnessStateRootLockName), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, errors.Wrap(err, "open state root lock")
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if closeErr := lock.Close(); closeErr != nil {
			return nil, false, errors.Wrapf(err, "close state root lock after claim failure: %v", closeErr)
		}
		if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "claim state root lock")
	}
	return lock, true, nil
}

func newHarnessStateRootOwner() (harnessStateRootOwner, error) {
	var token [harnessStateRootTokenBytes]byte
	if _, err := rand.Read(token[:]); err != nil {
		return harnessStateRootOwner{}, errors.Wrap(err, "create state root owner token")
	}
	return harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Now().UnixNano(),
		token:           hex.EncodeToString(token[:]),
	}, nil
}

func writeHarnessStateRootOwner(stateRoot string, owner harnessStateRootOwner) error {
	if err := os.WriteFile(filepath.Join(stateRoot, harnessStateRootOwnerName), marshalHarnessStateRootOwner(owner), 0o644); err != nil {
		return errors.Wrap(err, "write state root owner marker")
	}
	return nil
}

func marshalHarnessStateRootOwner(owner harnessStateRootOwner) []byte {
	var b strings.Builder
	b.WriteString(strconv.Itoa(owner.pid))
	b.WriteByte('\n')
	b.WriteString(strconv.FormatInt(owner.createdUnixNano, 10))
	b.WriteByte('\n')
	b.WriteString(owner.token)
	b.WriteByte('\n')
	return []byte(b.String())
}

func readHarnessStateRootOwner(stateRoot string) (harnessStateRootOwner, error) {
	data, err := os.ReadFile(filepath.Join(stateRoot, harnessStateRootOwnerName))
	if err != nil {
		return harnessStateRootOwner{}, err
	}
	return parseHarnessStateRootOwner(data)
}

func parseHarnessStateRootOwner(data []byte) (harnessStateRootOwner, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		return harnessStateRootOwner{}, errors.Errorf("state root owner marker has %d fields", len(lines))
	}
	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return harnessStateRootOwner{}, errors.Wrap(err, "parse state root owner pid")
	}
	createdUnixNano, err := strconv.ParseInt(lines[1], 10, 64)
	if err != nil {
		return harnessStateRootOwner{}, errors.Wrap(err, "parse state root owner created time")
	}
	token := strings.TrimSpace(lines[2])
	if len(token) != harnessStateRootTokenEncodedLength {
		return harnessStateRootOwner{}, errors.Errorf("state root owner token has %d bytes", len(token))
	}
	return harnessStateRootOwner{
		pid:             pid,
		createdUnixNano: createdUnixNano,
		token:           token,
	}, nil
}

func reapHarnessCacheOffStateRoots(le *logrus.Entry, parent, currentStateRoot, stableStateRoot string, currentOwner harnessStateRootOwner) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		if !os.IsNotExist(err) && le != nil {
			le.WithError(err).WithField("state-root-parent", parent).Warn("scan e2e wasm state roots")
		}
		return
	}
	now := time.Now()
	currentStateRoot = filepath.Clean(currentStateRoot)
	stableStateRoot = filepath.Clean(stableStateRoot)
	for _, entry := range entries {
		if !entry.IsDir() || !isHarnessCacheOffStateRootName(entry.Name()) {
			continue
		}
		stateRoot := filepath.Join(parent, entry.Name())
		cleanStateRoot := filepath.Clean(stateRoot)
		if cleanStateRoot == currentStateRoot || cleanStateRoot == stableStateRoot {
			continue
		}
		remove, err := shouldReapHarnessCacheOffStateRoot(stateRoot, entry, now, currentOwner)
		if err != nil {
			if le != nil {
				le.WithError(err).WithField("state-root", stateRoot).Warn("inspect e2e wasm state root")
			}
			continue
		}
		if !remove {
			continue
		}
		if err := os.RemoveAll(stateRoot); err != nil && le != nil {
			le.WithError(err).WithField("state-root", stateRoot).Warn("reap e2e wasm state root")
		}
	}
}

func shouldReapHarnessCacheOffStateRoot(stateRoot string, entry os.DirEntry, now time.Time, currentOwner harnessStateRootOwner) (bool, error) {
	owner, err := readHarnessStateRootOwner(stateRoot)
	if err == nil {
		// The marker token is kept in the live Harness state-root marker and
		// disambiguates stale roots that claim this process's PID after PID
		// reuse. A stale root whose PID has been recycled by another live
		// process is preserved; portable start-time checks are not available
		// across this harness's supported darwin/linux test hosts, so liveness
		// wins over cleanup.
		if owner.pid == currentOwner.pid && owner.token != currentOwner.token {
			return true, nil
		}
		return !harnessStateRootOwnerAlive(owner.pid), nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	// Pre-fix cache-off roots have no state-root marker. The 24h threshold only
	// reaps stale markerless wasm roots after the stable cache-on root name has
	// been excluded; young markerless roots may belong to an older live binary.
	return now.Sub(info.ModTime()) > harnessMarkerlessStateRootMaxAge, nil
}

func harnessStateRootOwnerAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || err == unix.EPERM
}

func isHarnessCacheOffStateRootName(name string) bool {
	if len(name) != len("wasm-")+8 || !strings.HasPrefix(name, "wasm-") {
		return false
	}
	for _, r := range name[len("wasm-"):] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// buildHarnessStateRoot returns a per-package harness state root.
//
// Recursive `go test ./e2e/wasm/...` runs boot multiple test binaries in
// parallel. They must not share the same `.bldr/e2e-wasm` directory or one
// package can delete `src/` while another is syncing it.
func buildHarnessStateRoot(repoRoot string, preserveStartupBuildCache bool) (string, error) {
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
	tokenInput := scope + "|" + filepath.Base(exe)
	if !preserveStartupBuildCache {
		// Cache-disabled runs should not share a devtool DB. Concurrent same
		// package e2e boots otherwise delete or close each other's state root.
		tokenInput += "|" + exe + "|" + strconv.Itoa(os.Getpid())
	}
	sum := sha256.Sum256([]byte(tokenInput))
	token := hex.EncodeToString(sum[:4])
	return filepath.Join(stateRoot, label+"-"+token), nil
}
