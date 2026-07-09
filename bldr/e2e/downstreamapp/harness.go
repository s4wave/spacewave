//go:build !js

package downstreamapp

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/devtool"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/sirupsen/logrus"
)

const (
	RunEnv             = "RUN_DOWNSTREAM_APP_E2E"
	CompilerEnv        = "BLDR_DOWNSTREAM_APP_COMPILER"
	WorkerModeEnv      = "BLDR_DOWNSTREAM_APP_WORKER_MODE"
	legacyCompilerEnv  = "BLDR_GO_PLUGIN_COMPILER_MODE"
	fixtureProjectPath = "bldr/e2e/downstreamapp/testdata/app/bldr.star"

	defaultManifestBuildTimeout = 5 * time.Minute
)

type BrowserCompiler string

const (
	BrowserCompilerGo       BrowserCompiler = "go"
	BrowserCompilerGoScript BrowserCompiler = "goscript"
)

type WorkerMode string

const (
	WorkerModeDedicated WorkerMode = "dedicated"
	WorkerModeShared    WorkerMode = "shared"
)

type Harness struct {
	ctx     context.Context
	cancel  context.CancelFunc
	le      *logrus.Entry
	devtool *devtool.DevtoolBus
	ref     directive.Reference

	projConfig    *bldr_project.ProjectConfig
	manifestRefs  []directive.Reference
	manifestWaits []manifestWait

	baseURL      string
	done         chan struct{}
	runErr       error
	restore      func()
	pw           *playwright.Playwright
	browser      playwright.Browser
	bootTime     time.Duration
	manifestWait time.Duration
	workerMode   WorkerMode
}

func ResolveBrowserCompiler() (BrowserCompiler, error) {
	if raw := strings.TrimSpace(os.Getenv(legacyCompilerEnv)); raw != "" {
		return "", errors.Errorf("%s is no longer supported; use %s=goscript", legacyCompilerEnv, CompilerEnv)
	}

	raw := strings.ToLower(strings.TrimSpace(os.Getenv(CompilerEnv)))
	switch raw {
	case "", string(BrowserCompilerGoScript):
		return BrowserCompilerGoScript, nil
	case string(BrowserCompilerGo):
		return BrowserCompilerGo, nil
	default:
		return "", errors.Errorf("unsupported %s value %q, expected go or goscript", CompilerEnv, raw)
	}
}

func ResolveWorkerMode() (WorkerMode, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(WorkerModeEnv)))
	switch raw {
	case "", string(WorkerModeDedicated), "dedicated-worker", "dedicated_workers":
		return WorkerModeDedicated, nil
	case string(WorkerModeShared), "shared-worker", "sharedworker":
		return WorkerModeShared, nil
	default:
		return "", errors.Errorf("unsupported %s value %q, expected dedicated or shared", WorkerModeEnv, raw)
	}
}

func Boot(ctx context.Context, le *logrus.Entry) (_ *Harness, retErr error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}
	stateRoot := filepath.Join(repoRoot, ".tmp", "bldr-downstreamapp-e2e")
	if err := os.RemoveAll(stateRoot); err != nil {
		return nil, errors.Wrap(err, "clear state root")
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, errors.Wrap(err, "create state root")
	}

	compiler, err := ResolveBrowserCompiler()
	if err != nil {
		return nil, err
	}
	restore, err := applyCompilerEnv(compiler)
	if err != nil {
		return nil, err
	}
	workerMode, err := ResolveWorkerMode()
	if err != nil {
		return nil, err
	}

	started := time.Now()
	hctx, cancel := context.WithCancel(ctx)
	h := &Harness{
		ctx:          hctx,
		cancel:       cancel,
		le:           le,
		done:         make(chan struct{}),
		restore:      restore,
		manifestWait: defaultManifestBuildTimeout,
		workerMode:   workerMode,
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
	if err := d.SyncDistSources("", "", repoRoot); err != nil {
		return nil, errors.Wrap(err, "sync dist sources")
	}

	projConfig, err := loadFixtureProjectConfig(repoRoot)
	if err != nil {
		return nil, err
	}
	if projConfig.Remotes == nil {
		projConfig.Remotes = make(map[string]*bldr_project.RemoteConfig)
	}
	projConfig.Remotes["devtool"] = &bldr_project.RemoteConfig{
		EngineId:       d.GetWorldEngineID(),
		PeerId:         d.GetVolume().GetPeerID().String(),
		ObjectKey:      d.GetPluginHostObjectKey(),
		LinkObjectKeys: []string{d.GetPluginHostObjectKey()},
	}
	if err := projConfig.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate fixture project config")
	}
	h.projConfig = projConfig

	projCtrlConf := bldr_project_controller.NewConfig(repoRoot, stateRoot, projConfig, false, false)
	projCtrlConf.FetchManifestRemote = "devtool"
	_, _, ref, err := loader.WaitExecControllerRunning(
		hctx,
		d.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start project controller")
	}
	h.ref = ref

	port, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find free port")
	}
	addr := "127.0.0.1:" + strconv.Itoa(port)
	h.baseURL = "http://" + addr

	webStartupSrcPath, _ := projConfig.GetStart().ParseWebStartupPath()
	go func() {
		h.runErr = d.ExecuteWebWasm(
			hctx,
			repoRoot,
			false,
			true,
			addr,
			projConfig.GetId(),
			projConfig.GetStart().GetPlugins(),
			devtool.ProjectOwnedStartupManifestPreflights(projConfig, "web/js/wasm"),
			webStartupSrcPath,
			workerMode == WorkerModeDedicated,
		)
		close(h.done)
	}()
	if err := h.waitForReady(hctx); err != nil {
		return nil, errors.Wrap(err, "wait for wasm readiness")
	}
	if err := h.enableBrowserReleaseAutoStart(); err != nil {
		return nil, errors.Wrap(err, "enable browser release auto-start")
	}
	if err := h.preflightStartupManifests(hctx); err != nil {
		return nil, errors.Wrap(err, "preflight startup manifests")
	}
	if err := h.preflightStartupManifests(hctx); err != nil {
		return nil, errors.Wrap(err, "settle startup manifests")
	}
	h.bootTime = time.Since(started)
	return h, nil
}

func loadFixtureProjectConfig(repoRoot string) (*bldr_project.ProjectConfig, error) {
	result, err := bldr_project_starlark.Evaluate(filepath.Join(repoRoot, fixtureProjectPath))
	if err != nil {
		return nil, errors.Wrap(err, "evaluate fixture bldr.star")
	}
	return result.Config, nil
}

func applyCompilerEnv(compiler BrowserCompiler) (func(), error) {
	prev, hadPrev := os.LookupEnv(gocompiler.GoCompilerEnv)
	if compiler == BrowserCompilerGoScript {
		if err := os.Setenv(gocompiler.GoCompilerEnv, string(gocompiler.GoCompilerGoScript)); err != nil {
			return nil, err
		}
	} else if hadPrev {
		if err := os.Unsetenv(gocompiler.GoCompilerEnv); err != nil {
			return nil, err
		}
	}
	return func() {
		if hadPrev {
			_ = os.Setenv(gocompiler.GoCompilerEnv, prev)
			return
		}
		_ = os.Unsetenv(gocompiler.GoCompilerEnv)
	}, nil
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

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
		case <-h.done:
			if h.runErr != nil {
				return errors.Wrap(h.runErr, "wasm lifecycle failed during startup")
			}
			return errors.New("wasm lifecycle exited before server became ready")
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (h *Harness) enableBrowserReleaseAutoStart() error {
	entryDir := filepath.Join(h.devtool.GetStateRoot(), "entry", "web", "wasm")
	return enableBrowserReleaseAutoStart(entryDir)
}

func enableBrowserReleaseAutoStart(entryDir string) error {
	descriptorPath := filepath.Join(entryDir, "browser-release.json")
	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		return err
	}
	var parser fastjson.Parser
	descriptor, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	descriptor.GetObject().Set("autoStart", fastjson.MustParse("true"))
	data = descriptor.MarshalTo(nil)
	data = append(data, '\n')
	return os.WriteFile(descriptorPath, data, 0o644)
}

type manifestFetchRequest struct {
	pluginID    string
	platformIDs []string
}

type manifestWait struct {
	req  manifestFetchRequest
	done <-chan error
}

func (r manifestFetchRequest) directive() directive.Directive {
	return bldr_manifest.NewFetchManifest(r.pluginID, nil, r.platformIDs, 0)
}

func (r manifestFetchRequest) summary() string {
	return r.pluginID + "[" + strings.Join(r.platformIDs, ",") + "]"
}

func (r manifestFetchRequest) logFields() logrus.Fields {
	return logrus.Fields{
		"plugin":   r.pluginID,
		"platform": strings.Join(r.platformIDs, ","),
	}
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

func (h *Harness) preflightStartupManifests(ctx context.Context) error {
	h.releaseManifestFetches()
	if err := h.assertStartupManifestFetches(); err != nil {
		return errors.Wrap(err, "assert startup manifest fetches")
	}
	if err := h.waitForManifests(ctx); err != nil {
		return errors.Wrap(err, "wait for manifest builds")
	}
	return nil
}

func (h *Harness) assertStartupManifestFetches() error {
	b := h.devtool.GetBus()
	for _, req := range h.startupManifestRequests() {
		if _, ok := h.projConfig.GetManifests()[req.pluginID]; !ok {
			return errors.Errorf("startup manifest %q not found in project config", req.pluginID)
		}
		waitState := newManifestWaitState(req.pluginID)
		di, ref, err := b.AddDirective(req.directive(), waitState.handler())
		if err != nil {
			return errors.Wrapf(err, "assert manifest fetch for %s", req.pluginID)
		}
		di.AddIdleCallback(waitState.handleIdle)
		h.manifestRefs = append(h.manifestRefs, ref)
		h.manifestWaits = append(h.manifestWaits, manifestWait{req: req, done: waitState.done})
	}
	return nil
}

func (h *Harness) waitForManifests(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, h.manifestWait)
	defer cancel()

	for _, wait := range h.manifestWaits {
		h.le.WithFields(wait.req.logFields()).Info("waiting for manifest build")
		select {
		case err := <-wait.done:
			if err != nil {
				return err
			}
			h.le.WithFields(wait.req.logFields()).Info("manifest build ready")
		case <-waitCtx.Done():
			if waitCtx.Err() == context.DeadlineExceeded {
				return errors.Errorf(
					"timed out after %s waiting for startup manifest callbacks: %s",
					h.manifestWait,
					h.startupManifestSummary(),
				)
			}
			return waitCtx.Err()
		}
	}
	return nil
}

func (h *Harness) startupManifestSummary() string {
	parts := make([]string, 0, len(h.manifestWaits))
	for _, wait := range h.manifestWaits {
		parts = append(parts, wait.req.summary())
	}
	return strings.Join(parts, ", ")
}

func (h *Harness) releaseManifestFetches() {
	for _, ref := range h.manifestRefs {
		ref.Release()
	}
	h.manifestRefs = nil
	h.manifestWaits = nil
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

func (h *Harness) LaunchBrowser() error {
	pw, err := playwright.Run()
	if err != nil {
		return errors.Wrap(err, "start playwright")
	}
	h.pw = pw
	headless := true
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
		Args: []string{
			"--allow-loopback-in-peer-connection",
			"--disable-features=WebRtcHideLocalIpsWithMdns",
		},
	})
	if err != nil {
		pw.Stop()
		h.pw = nil
		return errors.Wrap(err, "launch chromium")
	}
	h.browser = browser
	return nil
}

func (h *Harness) NewPage() (playwright.BrowserContext, playwright.Page, error) {
	ctx, err := h.browser.NewContext()
	if err != nil {
		return nil, nil, errors.Wrap(err, "new browser context")
	}
	page, err := ctx.NewPage()
	if err != nil {
		ctx.Close()
		return nil, nil, errors.Wrap(err, "new page")
	}
	return ctx, page, nil
}

func (h *Harness) BaseURL() string { return h.baseURL }

func (h *Harness) BootTime() time.Duration { return h.bootTime }

func (h *Harness) Release() {
	if h.browser != nil {
		_ = h.browser.Close()
	}
	if h.pw != nil {
		_ = h.pw.Stop()
	}
	if h.cancel != nil {
		h.cancel()
	}
	if h.done != nil {
		<-h.done
	}
	h.releaseManifestFetches()
	if h.ref != nil {
		h.ref.Release()
	}
	if h.devtool != nil {
		h.devtool.Release()
	}
	if h.restore != nil {
		h.restore()
	}
}
