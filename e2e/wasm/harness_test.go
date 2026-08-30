//go:build !skip_e2e && !js

package wasm

import (
	"bytes"
	"context"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	space "github.com/s4wave/spacewave/core/space"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	e2e_wasm_session "github.com/s4wave/spacewave/e2e/wasm/session"
	forge_job "github.com/s4wave/spacewave/forge/job"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
	exptrace "golang.org/x/exp/trace"
)

// sharedHarness is the package-level harness, booted lazily on first access via
// harness(). It owns the devtool bus, WASM build, HTTP server, and Playwright
// browser process. Booting is deferred so test slices that exercise only
// pure-unit logic (config resolution, crash classifier, peer watcher, script
// compilation) never pay the multi-minute Manifest build. Individual tests
// create isolated sessions via h.NewCleanSession(t).
var (
	harnessOnce    sync.Once
	sharedHarness  *Harness
	harnessBootErr error
)

// TIER: pr
func TestMain(m *testing.M) {
	if !E2EWasmEnabled() && !e2eWasmPureUnitRunWithoutHarness() {
		logrus.NewEntry(logrus.New()).Info("skipping e2e/wasm package; set ENABLE_E2E_WASM=true to run")
		os.Exit(0)
	}

	code := m.Run()
	if sharedHarness != nil {
		sharedHarness.Release()
	}
	os.Exit(code)
}

func e2eWasmPureUnitRunWithoutHarness() bool {
	if !flag.Parsed() {
		flag.Parse()
	}
	runFlag := flag.Lookup("test.run")
	if runFlag == nil {
		return false
	}
	return runFlag.Value.String() == "Reap|StateRoot|Marker"
}

// harness returns the package-level shared harness, booting it on first use.
// A boot failure fails the calling test rather than aborting the whole package,
// so harness-independent tests in the same slice still run.
func harness(t testing.TB) *Harness {
	t.Helper()
	harnessOnce.Do(func() {
		sharedHarness, harnessBootErr = bootSharedHarness()
	})
	if harnessBootErr != nil {
		t.Fatalf("boot wasm harness: %v", harnessBootErr)
	}
	return sharedHarness
}

// sharedHarnessBooted reports whether the package harness has already been
// built. A test that needs the harness compiled a particular way checks this
// before configuring the build environment, since harnessOnce will not rebuild
// what an earlier test in the same slice already built.
func sharedHarnessBooted() bool {
	return sharedHarness != nil || harnessBootErr != nil
}

// bootSharedHarness boots the harness, launches the browser, and compiles the
// e2e test scripts. It runs once, guarded by harnessOnce.
func bootSharedHarness() (*Harness, error) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	ctx := context.Background()

	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		return nil, errors.Wrap(err, "configure e2e wasm compiler")
	}
	opts := []Option{
		WithSessionHarness(),
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_E2E")), "true") {
		opts = append(opts, WithManifestBuildTimeout(15*time.Minute))
		opts = append(opts, WithConfigMutator(func(projectConfig *bldr_project.ProjectConfig) error {
			start := projectConfig.GetStart()
			if !slices.Contains(start.Plugins, "spacewave-v86") {
				start.Plugins = append(start.Plugins, "spacewave-v86")
			}
			return nil
		}))
	}
	if compiler != E2EWasmCompilerGo {
		manifestBuildTimeout, err := ResolveE2EWasmManifestBuildTimeout(20 * time.Minute)
		if err != nil {
			return nil, errors.Wrap(err, "configure e2e wasm manifest build timeout")
		}
		opts = append(opts, WithManifestBuildTimeout(manifestBuildTimeout))
	}

	switch compiler {
	case E2EWasmCompilerGo:
	case E2EWasmCompilerTinyGo:
		if err := ApplyE2EWasmTinyGoCompilerEnv(); err != nil {
			return nil, errors.Wrap(err, "configure TinyGo e2e wasm compiler env")
		}
		opts = append(opts, WithTinyGoCore())
	case E2EWasmCompilerGoScript:
		opts = append(opts, WithGoScriptBrowserStartup())
	}

	// Inject the trace service after the compiler mutators so the GoScript
	// startup override, which rebuilds the launcher and core manifests, cannot
	// drop it from the final config.
	if E2EWasmTraceServiceEnabled(compiler) {
		opts = append(opts, WithConfigMutator(trace_service.InjectTraceConfig))
	} else {
		le.WithField("compiler", compiler).Info("trace service injection disabled for this e2e/wasm compiler mode")
	}

	h, err := Boot(ctx, le, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "boot wasm harness")
	}

	if err := h.LaunchBrowser(); err != nil {
		h.Release()
		return nil, errors.Wrap(err, "launch browser")
	}

	if err := h.CompileScripts("."); err != nil {
		h.Release()
		return nil, errors.Wrap(err, "compile test scripts")
	}

	return h, nil
}

// TestWasmHarnessBoot verifies the shared harness is serving.
func TestWasmHarnessBoot(t *testing.T) {
	h := harness(t)
	if h.Port() == 0 {
		t.Fatal("expected non-zero port")
	}

	resp, err := http.Get(h.BaseURL() + "/bldr-dev/web-wasm/info")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestWasmHarnessTraceConfig verifies trace service wiring was injected into
// every Go builder manifest by InjectTraceConfig.
func TestWasmHarnessTraceConfig(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	h := harness(t)
	for name, manifest := range h.GetProjectConfig().GetManifests() {
		builder := manifest.GetBuilder()
		if builder == nil || builder.GetId() != bldr_plugin_compiler_go.ConfigID {
			continue
		}

		goConf := &bldr_plugin_compiler_go.Config{}
		if data := builder.GetConfig(); len(data) != 0 {
			if err := goConf.UnmarshalJSON(data); err != nil {
				t.Fatalf("unmarshal %s builder config: %v", name, err)
			}
		}

		found := slices.Contains(goConf.GetGoPkgs(), "./core/trace/service")
		if !found {
			t.Fatalf("manifest %s missing ./core/trace/service in goPkgs", name)
		}

		if _, ok := goConf.GetConfigSet()["trace-service"]; !ok {
			t.Fatalf("manifest %s missing trace-service in configSet", name)
		}
	}
}

func skipTraceServiceWhenDisabled(t testing.TB) {
	t.Helper()
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if !E2EWasmTraceServiceEnabled(compiler) {
		t.Skipf("trace service is disabled in %s e2e/wasm mode", compiler)
	}
}

// TestSessionHarnessPeerInfo verifies the session harness controller is
// running in the browser WASM by calling GetPeerInfo and asserting a
// non-empty peer ID.
func TestSessionHarnessPeerInfo(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	client := sess.BrowserClient()
	if client == nil {
		t.Fatal("expected non-nil browser client")
	}

	ctx := harness(t).Context()
	peerInfoClient := newPeerInfoClient(sess)
	resp, err := peerInfoClient.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo: %v", err)
	}
	if resp.GetPeerId() == "" {
		t.Fatal("expected non-empty peer ID from session harness")
	}
	t.Logf("session harness peer ID: %s", resp.GetPeerId())
}

const quicRwcFixtureDeadline = 45 * time.Second

// TestBrowserWorkerQuicRwcFixture verifies both QUIC roles can handshake and
// exchange a stream payload over detached RTCDataChannels transferred into one
// browser worker.
func TestBrowserWorkerQuicRwcFixture(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler == E2EWasmCompilerTinyGo {
		t.Skip("browser QUIC fixture requires a compiler with pion/webrtc support")
	}
	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(harness(t).Context(), quicRwcFixtureDeadline)
	defer cancel()

	payload := []byte("spacewave-quic-rwc-fixture")
	type fixtureResult struct {
		resp *e2e_wasm_session.RunQuicRwcFixtureResponse
		err  error
	}
	resultCh := make(chan fixtureResult, 1)
	client := newQuicRwcFixtureClient(sess)
	go func() {
		resp, err := client.RunQuicRwcFixture(ctx, &e2e_wasm_session.RunQuicRwcFixtureRequest{
			Payload: payload,
		})
		resultCh <- fixtureResult{resp: resp, err: err}
	}()

	lastPhase := "RPC dispatch"
	consoleCh, stopConsole := sess.WatchConsole()
	defer stopConsole()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf(
				"no QUIC progress past %s within %s: %v",
				lastPhase, quicRwcFixtureDeadline, ctx.Err(),
			)
		case msg, ok := <-consoleCh:
			if !ok {
				consoleCh = nil
				continue
			}
			if strings.Contains(msg, "quic fixture phase:") {
				lastPhase = msg
				t.Logf("QUIC fixture progress: %s", msg)
			}
		case result := <-resultCh:
			if result.err != nil {
				t.Fatalf("QUIC fixture failed after %s: %v", lastPhase, result.err)
			}
			if !bytes.Equal(result.resp.GetEchoedPayload(), payload) {
				t.Fatalf("QUIC fixture echo = %q, want %q", result.resp.GetEchoedPayload(), payload)
			}
			return
		}
	}
}

// TestMultiSessionPeerDiscovery verifies two browser sessions produce
// distinct bifrost peers discoverable via the session harness.
func TestMultiSessionPeerDiscovery(t *testing.T) {
	sessA := harness(t).NewCleanSession(t)
	sessB := harness(t).NewCleanSession(t)

	ctx, cancel := context.WithCancel(harness(t).Context())
	t.Cleanup(cancel)
	clientA := newPeerInfoClient(sessA)
	clientB := newPeerInfoClient(sessB)

	respA, err := clientA.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo A: %v", err)
	}
	respB, err := clientB.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo B: %v", err)
	}

	if respA.GetPeerId() == "" || respB.GetPeerId() == "" {
		t.Fatal("expected non-empty peer IDs from both sessions")
	}
	if respA.GetPeerId() == respB.GetPeerId() {
		t.Fatal("expected distinct peer IDs from two sessions")
	}
	t.Logf("session A peer: %s, session B peer: %s", respA.GetPeerId(), respB.GetPeerId())
}

// TestSignalRelayCrossConnect verifies two sessions can open SignalRelay
// streams targeting each other and forward messages through the Go test
// process.
func TestSignalRelayCrossConnect(t *testing.T) {
	sessA := harness(t).NewCleanSession(t)
	sessB := harness(t).NewCleanSession(t)

	ctx, cancel := context.WithCancel(harness(t).Context())
	t.Cleanup(cancel)
	peerInfoA := newPeerInfoClient(sessA)
	peerInfoB := newPeerInfoClient(sessB)

	respA, err := peerInfoA.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo A: %v", err)
	}
	respB, err := peerInfoB.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo B: %v", err)
	}
	t.Logf("peer A: %s, peer B: %s", respA.GetPeerId(), respB.GetPeerId())

	// Open SignalRelay streams: A targets B, B targets A.
	relayA := newSignalRelayClient(sessA)
	relayB := newSignalRelayClient(sessB)

	strmA, err := relayA.SignalRelay(ctx)
	if err != nil {
		t.Fatalf("SignalRelay A: %v", err)
	}
	strmB, err := relayB.SignalRelay(ctx)
	if err != nil {
		t.Fatalf("SignalRelay B: %v", err)
	}

	// Send init messages: A says "I want to relay for peer B", B for peer A.
	if err := strmA.Send(&e2e_wasm_session.SignalRelayMessage{
		Body: &e2e_wasm_session.SignalRelayMessage_Init{
			Init: &e2e_wasm_session.SignalRelayInit{RemotePeerId: respB.GetPeerId()},
		},
	}); err != nil {
		t.Fatalf("send init A: %v", err)
	}
	if err := strmB.Send(&e2e_wasm_session.SignalRelayMessage{
		Body: &e2e_wasm_session.SignalRelayMessage_Init{
			Init: &e2e_wasm_session.SignalRelayInit{RemotePeerId: respA.GetPeerId()},
		},
	}); err != nil {
		t.Fatalf("send init B: %v", err)
	}

	// Start cross-connect forwarding.
	_, errCh := RelayCrossConnect(ctx, strmA, strmB)

	// The cross-connect goroutines are now running. If they fail immediately,
	// catch the error. Otherwise the test succeeds (relay is wired).
	select {
	case err := <-errCh:
		// Only fail if not caused by context cancellation.
		if ctx.Err() == nil {
			t.Fatalf("relay cross-connect error: %v", err)
		}
	default:
		t.Log("relay cross-connect established successfully")
	}
}

const linkEstablishmentDeadline = 45 * time.Second

// TestEndToEndLinkEstablishment verifies two browser WASM sessions can
// establish a bifrost link through the signaling relay cross-connect.
func TestEndToEndLinkEstablishment(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if !e2eWasmBrowserWebRTCEnabled(compiler) {
		t.Skipf("requires browser WebRTC transport support; compiler=%s", compiler)
	}

	sessA := harness(t).NewCleanSession(t)
	sessB := harness(t).NewCleanSession(t)

	ctx, cancel := context.WithCancel(harness(t).Context())
	t.Cleanup(cancel)
	peerInfoA := newPeerInfoClient(sessA)
	peerInfoB := newPeerInfoClient(sessB)

	respA, err := peerInfoA.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo A: %v", err)
	}
	respB, err := peerInfoB.GetPeerInfo(ctx, &e2e_wasm_session.GetPeerInfoRequest{})
	if err != nil {
		t.Fatalf("GetPeerInfo B: %v", err)
	}
	t.Logf("peer A: %s, peer B: %s", respA.GetPeerId(), respB.GetPeerId())

	relayA := newSignalRelayClient(sessA)
	relayB := newSignalRelayClient(sessB)
	strmA, err := relayA.SignalRelay(ctx)
	if err != nil {
		t.Fatalf("SignalRelay A: %v", err)
	}
	strmB, err := relayB.SignalRelay(ctx)
	if err != nil {
		t.Fatalf("SignalRelay B: %v", err)
	}

	if err := strmA.Send(&e2e_wasm_session.SignalRelayMessage{
		Body: &e2e_wasm_session.SignalRelayMessage_Init{
			Init: &e2e_wasm_session.SignalRelayInit{RemotePeerId: respB.GetPeerId()},
		},
	}); err != nil {
		t.Fatalf("send init A: %v", err)
	}
	if err := strmB.Send(&e2e_wasm_session.SignalRelayMessage{
		Body: &e2e_wasm_session.SignalRelayMessage_Init{
			Init: &e2e_wasm_session.SignalRelayInit{RemotePeerId: respA.GetPeerId()},
		},
	}); err != nil {
		t.Fatalf("send init B: %v", err)
	}

	watchCtx, watchCancel := context.WithTimeout(ctx, linkEstablishmentDeadline)
	t.Cleanup(watchCancel)
	relayProgressCh, relayErrCh := RelayCrossConnect(watchCtx, strmA, strmB)

	consoleA, stopConsoleA := sessA.WatchConsole()
	consoleB, stopConsoleB := sessB.WatchConsole()
	t.Cleanup(stopConsoleA)
	t.Cleanup(stopConsoleB)
	quicProgressCh := make(chan string, 16)
	forwardQuicProgress := func(side string, messages <-chan string) {
		for {
			select {
			case <-watchCtx.Done():
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}
				if !strings.Contains(msg, "webrtc quic phase:") {
					continue
				}
				select {
				case quicProgressCh <- side + ": " + msg:
				case <-watchCtx.Done():
					return
				}
			}
		}
	}
	go forwardQuicProgress("A", consoleA)
	go forwardQuicProgress("B", consoleB)

	linkClient := newEstablishLinkClient(sessA)
	watchStrm, err := linkClient.WatchState(watchCtx, &e2e_wasm_session.WatchStateRequest{
		TargetPeerId: respB.GetPeerId(),
	})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}

	type watchResult struct {
		resp *e2e_wasm_session.WatchStateResponse
		err  error
	}
	watchResultCh := make(chan watchResult, 1)
	recvState := func() {
		resp, err := watchStrm.Recv()
		watchResultCh <- watchResult{resp: resp, err: err}
	}
	go recvState()

	lastPhase := "WatchState started"
	var relayAB, relayBA RelayProgress
	for {
		select {
		case <-watchCtx.Done():
			t.Fatalf(
				"no QUIC progress past %s within %s; relay A->B=%d messages/%d bytes, B->A=%d messages/%d bytes",
				lastPhase,
				linkEstablishmentDeadline,
				relayAB.Messages,
				relayAB.Bytes,
				relayBA.Messages,
				relayBA.Bytes,
			)
		case relayErr := <-relayErrCh:
			if watchCtx.Err() != nil {
				continue
			}
			t.Fatalf("relay cross-connect error after %s: %v", lastPhase, relayErr)
		case progress := <-relayProgressCh:
			switch progress.Direction {
			case "A->B":
				relayAB = progress
			case "B->A":
				relayBA = progress
			}
			t.Logf(
				"signaling relay %s: %d messages, %d bytes",
				progress.Direction,
				progress.Messages,
				progress.Bytes,
			)
		case phase := <-quicProgressCh:
			lastPhase = phase
			t.Logf("QUIC progress: %s", phase)
		case result := <-watchResultCh:
			if result.err != nil {
				if watchCtx.Err() != nil {
					watchResultCh = nil
					continue
				}
				t.Fatalf("WatchState recv after %s: %v", lastPhase, result.err)
			}

			state := result.resp.GetState()
			t.Logf("link state: %s", state.String())
			switch state {
			case e2e_wasm_session.EstablishLinkState_EstablishLinkState_CONNECTED:
				t.Log("bifrost link established between two browser sessions")
				return
			case e2e_wasm_session.EstablishLinkState_EstablishLinkState_FAILED:
				t.Fatalf("link establishment failed after %s", lastPhase)
			default:
				lastPhase = state.String()
				go recvState()
			}
		}
	}
}

// TestWasmHarnessPackageLifecycle verifies the shared harness is reused
// across tests rather than booting a new instance per test.
func TestWasmHarnessPackageLifecycle(t *testing.T) {
	if harness(t) == nil {
		t.Fatal("expected shared harness from TestMain")
	}
	if harness(t).Port() == 0 {
		t.Fatal("shared harness has zero port")
	}
}

// TestWasmHarnessReadiness verifies the info endpoint responds immediately
// since Boot already waited for server readiness.
func TestWasmHarnessReadiness(t *testing.T) {
	h := harness(t)
	resp, err := http.Get(h.BaseURL() + "/bldr-dev/web-wasm/info")
	if err != nil {
		t.Fatalf("info endpoint unreachable after Boot: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if resp.ContentLength == 0 {
		t.Fatal("expected non-empty info response")
	}
}

// TestWasmHarnessTeardown verifies the harness is still usable at test time.
func TestWasmHarnessTeardown(t *testing.T) {
	h := harness(t)
	resp, err := http.Get(h.BaseURL() + "/bldr-dev/web-wasm/info")
	if err != nil {
		t.Fatalf("harness became unresponsive before teardown: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestBrowserLaunchFromGo verifies the browser process was launched.
func TestBrowserLaunchFromGo(t *testing.T) {
	h := harness(t)
	if h.Browser() == nil {
		t.Fatal("expected non-nil browser")
	}
	if !h.Browser().IsConnected() {
		t.Fatal("browser not connected")
	}
}

// TestBrowserSessionIsolation verifies each NewCleanSession creates a fresh
// browser context with clean storage while the devtool bus stays shared.
func TestBrowserSessionIsolation(t *testing.T) {
	h := harness(t)

	// First session: inject a localStorage marker.
	s1 := h.NewCleanSession(t)
	lsScript := h.Script("local-storage.ts")
	_, err := s1.Page().Evaluate(lsScript, map[string]any{
		"op": "set", "key": "test-marker", "value": "exists",
	})
	if err != nil {
		t.Fatalf("inject localStorage marker: %v", err)
	}

	// Second session: localStorage should be clean.
	s2 := h.NewCleanSession(t)
	val, err := s2.Page().Evaluate(lsScript, map[string]any{
		"op": "get", "key": "test-marker",
	})
	if err != nil {
		t.Fatalf("read localStorage: %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil localStorage marker in fresh session, got %v", val)
	}

	// The HTTP server should still be responsive.
	resp, err := http.Get(h.BaseURL() + "/bldr-dev/web-wasm/info")
	if err != nil {
		t.Fatalf("server unreachable after session switch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestRetainedStatePageSessionReusesWarmContext verifies the opt-in page-only
// mode reuses one warm BrowserContext while clean page sessions remain isolated.
func TestRetainedStatePageSessionReusesWarmContext(t *testing.T) {
	h := harness(t)
	lsScript := h.Script("local-storage.ts")
	markerKey := "retained-state-page-session-marker"

	s1 := h.NewRetainedStatePageSession(t)
	if s1.ownsBrowserCtx {
		t.Fatal("expected retained-state page session not to own browser context")
	}
	retainedCtx := s1.BrowserContext()
	firstPage := s1.Page()
	if _, err := firstPage.Evaluate(lsScript, map[string]any{
		"op": "set", "key": markerKey, "value": "exists",
	}); err != nil {
		t.Fatalf("set retained localStorage marker: %v", err)
	}

	s1.Release()
	if got := h.LookupSessionByPage(firstPage); got != nil {
		t.Fatal("expected released retained-state page to be unregistered")
	}

	s2 := h.NewRetainedStatePageSession(t)
	if s2.BrowserContext() != retainedCtx {
		t.Fatal("expected second retained-state page session to reuse warm context")
	}
	if s2.Page() == firstPage {
		t.Fatal("expected second retained-state page session to use a fresh page")
	}
	val, err := s2.Page().Evaluate(lsScript, map[string]any{
		"op": "get", "key": markerKey,
	})
	if err != nil {
		t.Fatalf("read retained localStorage marker: %v", err)
	}
	if val != "exists" {
		t.Fatalf("expected retained localStorage marker, got %v", val)
	}

	isolated := h.NewCleanPageSession(t)
	val, err = isolated.Page().Evaluate(lsScript, map[string]any{
		"op": "get", "key": markerKey,
	})
	if err != nil {
		t.Fatalf("read isolated localStorage marker: %v", err)
	}
	if val != nil {
		t.Fatalf("expected isolated page session to have clean storage, got %v", val)
	}
}

// TestRetainedStateResourceSessionSupportsSequentialReuse verifies the opt-in
// Resource SDK helper can run sequential sessions on the retained warm context.
func TestRetainedStateResourceSessionSupportsSequentialReuse(t *testing.T) {
	h := harness(t)

	s1 := h.NewRetainedStateSession(t)
	if s1.Root() == nil {
		t.Fatal("expected first retained-state session root resource")
	}
	retainedCtx := s1.BrowserContext()
	firstPeer := s1.browserPeer
	if len(firstPeer) == 0 {
		t.Fatal("expected first retained-state session browser peer")
	}

	s1.Release()

	s2 := h.NewRetainedStateSession(t)
	if s2.Root() == nil {
		t.Fatal("expected second retained-state session root resource")
	}
	if s2.BrowserContext() != retainedCtx {
		t.Fatal("expected second retained-state resource session to reuse warm context")
	}
	if len(s2.browserPeer) == 0 {
		t.Fatal("expected second retained-state session browser peer")
	}
	t.Logf(
		"retained-state resource session peers: first=%s second=%s reused=%t",
		firstPeer,
		s2.browserPeer,
		firstPeer == s2.browserPeer,
	)
}

// TestRetainedStateSessionReleaseCleansPerSessionState proves sequential
// retained-state sessions do not keep page registrations, console watchers,
// Resource SDK handles, or browser peer leases after release.
func TestRetainedStateSessionReleaseCleansPerSessionState(t *testing.T) {
	h := harness(t)
	baselinePages := h.pageSessionCount()
	baselineLeases := h.browserPeerLeaseCount()

	s1 := h.NewRetainedStateSession(t)
	firstPage := s1.Page()
	firstPeer := s1.browserPeer
	if firstPage == nil {
		t.Fatal("expected first retained-state session page")
	}
	if len(firstPeer) == 0 {
		t.Fatal("expected first retained-state session peer")
	}
	if got := h.LookupSessionByPage(firstPage); got != s1 {
		t.Fatal("expected first page registration to point at first session")
	}
	if got := h.browserPeerLeaseOwner(firstPeer); got != s1 {
		t.Fatal("expected first peer lease to point at first session")
	}
	if s1.Root() == nil || s1.ResourceClient() == nil || s1.BrowserClient() == nil {
		t.Fatal("expected first session resource handles")
	}

	stoppedConsole, stopStoppedConsole := s1.WatchConsole()
	releasedConsole, _ := s1.WatchConsole()
	if got := s1.consoleWatcherCount(); got != 2 {
		t.Fatalf("expected two console watchers, got %d", got)
	}
	stopStoppedConsole()
	assertConsoleClosed(t, stoppedConsole)
	if got := s1.consoleWatcherCount(); got != 1 {
		t.Fatalf("expected one console watcher after explicit stop, got %d", got)
	}

	s1.Release()
	assertConsoleClosed(t, releasedConsole)
	if got := h.LookupSessionByPage(firstPage); got != nil {
		t.Fatal("expected first page registration to be removed after release")
	}
	if got := h.browserPeerLeaseOwner(firstPeer); got != nil {
		t.Fatal("expected first peer lease to be removed after release")
	}
	if got := h.pageSessionCount(); got != baselinePages {
		t.Fatalf("expected page registrations to return to %d, got %d", baselinePages, got)
	}
	if got := h.browserPeerLeaseCount(); got != baselineLeases {
		t.Fatalf("expected peer leases to return to %d, got %d", baselineLeases, got)
	}
	if s1.Root() != nil || s1.ResourceClient() != nil || s1.BrowserClient() != nil {
		t.Fatal("expected first session resource handles to be cleared after release")
	}

	s2 := h.NewRetainedStateSession(t)
	secondPage := s2.Page()
	secondPeer := s2.browserPeer
	if secondPage == nil {
		t.Fatal("expected second retained-state session page")
	}
	if secondPage == firstPage {
		t.Fatal("expected second retained-state session to use a fresh page")
	}
	if len(secondPeer) == 0 {
		t.Fatal("expected second retained-state session peer")
	}
	if got := h.LookupSessionByPage(secondPage); got != s2 {
		t.Fatal("expected second page registration to point at second session")
	}
	if got := h.browserPeerLeaseOwner(secondPeer); got != s2 {
		t.Fatal("expected second peer lease to point at second session")
	}

	s2.Release()
	if got := h.LookupSessionByPage(secondPage); got != nil {
		t.Fatal("expected second page registration to be removed after release")
	}
	if got := h.browserPeerLeaseOwner(secondPeer); got != nil {
		t.Fatal("expected second peer lease to be removed after release")
	}
	if got := h.pageSessionCount(); got != baselinePages {
		t.Fatalf("expected page registrations to return to %d after second release, got %d", baselinePages, got)
	}
	if got := h.browserPeerLeaseCount(); got != baselineLeases {
		t.Fatalf("expected peer leases to return to %d after second release, got %d", baselineLeases, got)
	}
}

func assertConsoleClosed(t testing.TB, ch <-chan string) {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("expected console watcher channel to close")
		}
	}
}

// TestBrowserHelpersAndRawAccess verifies raw Playwright access works.
func TestBrowserHelpersAndRawAccess(t *testing.T) {
	sess := harness(t).NewCleanSession(t)

	page := sess.Page()
	err := page.Locator("body").WaitFor()
	if err != nil {
		t.Fatalf("WaitFor body: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("page.Content: %v", err)
	}
	if content == "" {
		t.Fatal("expected non-empty page content")
	}
}

// TestBrowserRouteNavigation verifies the session page loaded the app URL.
func TestBrowserRouteNavigation(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	h := harness(t)

	page := sess.Page()
	url := page.URL()
	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	if !strings.HasPrefix(url, h.BaseURL()) {
		t.Fatalf("page URL %q does not start with base %q", url, h.BaseURL())
	}
}

// TestRootResourceMount verifies the Resource SDK client is connected and
// can access the root resource within an isolated session.
func TestRootResourceMount(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	if sess.ResourceClient() == nil {
		t.Fatal("expected non-nil resource client")
	}
	root := sess.Root()
	if root == nil {
		t.Fatal("expected non-nil root")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	providers, err := root.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected at least one provider")
	}
}

// TestSessionMount verifies a session can be mounted from Go.
func TestSessionMount(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	sessions, err := sess.Root().ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Skip("no sessions configured, skipping session mount test")
	}

	s, err := sess.MountSessionByIdx(ctx, 1)
	if err != nil {
		t.Fatalf("MountSessionByIdx: %v", err)
	}
	defer s.Release()

	ref := s.GetResourceRef()
	if ref == nil {
		t.Fatal("expected non-nil session resource ref")
	}
}

// TestSpaceMountAfterQuickstart verifies state created through the browser
// app is visible to Go resource mounts.
func TestSpaceMountAfterQuickstart(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	sessions, err := sess.Root().ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Skip("no sessions, skipping space mount test")
	}

	s, err := sess.MountSessionByIdx(ctx, 1)
	if err != nil {
		t.Fatalf("MountSessionByIdx: %v", err)
	}
	defer s.Release()

	rlStream, err := s.WatchResourcesList(ctx)
	if err != nil {
		t.Fatalf("WatchResourcesList: %v", err)
	}
	resp, err := rlStream.Recv()
	if err != nil {
		t.Fatalf("WatchResourcesList recv: %v", err)
	}
	rlStream.Close()
	spaces := resp.GetSpacesList()
	if len(spaces) == 0 {
		t.Skip("no spaces, skipping space mount test")
	}
	t.Logf("found %d space(s)", len(spaces))
}

// TestResourceSetupHelpers verifies resource helpers work for setup and
// teardown outside of profiled interactions.
func TestResourceSetupHelpers(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	root := sess.Root()
	if root == nil {
		t.Fatal("expected non-nil root")
	}

	providers, err := root.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	t.Logf("setup helper found %d provider(s)", len(providers))
}

// TestTraceCaptureBytes verifies StartTrace and StopTrace capture a non-empty
// raw trace and return the bytes to the Go test process.
func TestTraceCaptureBytes(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if err := sess.StartTrace(ctx, "test-capture"); err != nil {
		t.Fatalf("StartTrace: %v", err)
	}

	// The WASM process has constant goroutine scheduling activity, so
	// trace events are produced without explicit user interaction.
	data, err := sess.StopTrace(ctx)
	if err != nil {
		t.Fatalf("StopTrace: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty trace data")
	}
	assertTraceParses(t, data)
	t.Logf("captured %d bytes of trace data", len(data))
}

// assertTraceParses walks the captured bytes with the upstream Go trace reader,
// the same parser tracetool's fork is built on, so a clean walk to EOF proves
// tracetool reads the GoScript capture.
func assertTraceParses(t testing.TB, data []byte) {
	t.Helper()
	reader, err := exptrace.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("upstream trace reader rejected the capture header: %v", err)
	}
	for {
		_, err := reader.ReadEvent()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("upstream trace reader failed mid-capture: %v", err)
		}
	}
}

// TestTraceCaptureWritesFile verifies the returned bytes are written to an
// explicit destination path owned by the Go test process.
func TestTraceCaptureWritesFile(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if err := sess.StartTrace(ctx, "write-file"); err != nil {
		t.Fatalf("StartTrace: %v", err)
	}
	data, err := sess.StopTrace(ctx)
	if err != nil {
		t.Fatalf("StopTrace: %v", err)
	}

	path := filepath.Join(t.TempDir(), "trace.out")
	if err := WriteTraceArtifact(path, data); err != nil {
		t.Fatalf("WriteTraceArtifact: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat trace file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty trace file")
	}
}

// TestTracePathDerivation verifies default artifact paths are derived beside
// the calling test, sanitized, and stable across repeated runs.
func TestTracePathDerivation(t *testing.T) {
	p := TraceArtifactPath(t)
	if p == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.Contains(p, "TestTracePathDerivation") {
		t.Fatalf("path %q does not contain test name", p)
	}
	p2 := TraceArtifactPath(t)
	if p != p2 {
		t.Fatalf("path not stable: %q vs %q", p, p2)
	}
	t.Run("sub/test", func(t *testing.T) {
		sp := TraceArtifactPath(t)
		if strings.Contains(sp, "/sub/") {
			t.Fatalf("subtest path not sanitized: %q", sp)
		}
	})
}

// TestTraceWindowControl verifies trace helpers can bracket only the profiled
// interaction instead of full app boot.
func TestTraceWindowControl(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	data, err := sess.CaptureTrace(ctx, "window-control", func(ctx context.Context) error {
		// The WASM process has constant goroutine scheduling activity,
		// so trace events are produced without explicit user interaction.
		return nil
	})
	if err != nil {
		t.Fatalf("CaptureTrace: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty trace from bracketed capture")
	}
	t.Logf("bracketed capture: %d bytes", len(data))
}

// TestTracePolicyBehavior verifies trace capture behavior: discard-on-replace,
// no watchdog, no forced timeout.
func TestTracePolicyBehavior(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if err := sess.StartTrace(ctx, "first"); err != nil {
		t.Fatalf("StartTrace first: %v", err)
	}
	if err := sess.StartTrace(ctx, "second"); err != nil {
		t.Fatalf("StartTrace second (replace): %v", err)
	}

	data, err := sess.StopTrace(ctx)
	if err != nil {
		t.Fatalf("StopTrace: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty trace after replace")
	}
}

// TestGoScriptQuickstartDriveDirectRouteMountGate verifies the GoScript direct
// Drive route reaches the mounted file browser, not only quickstart
// content-ready.
func TestGoScriptQuickstartDriveDirectRouteMountGate(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve e2e wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript-only regression gate; compiler=%s", compiler)
	}

	sess := harness(t).NewCleanBlankSession(t)
	script := "globalThis.__s4waveLogQuickstartTiming = true;"
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install quickstart timing init script: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during direct drive mount gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during direct drive mount gate: %+v", report)
		}
	}()

	if err := harness(t).loadAppPageURL(sess, harness(t).baseURL+"/#/quickstart/drive"); err != nil {
		t.Fatalf("load direct drive route: %v", err)
	}
	page := sess.Page()
	WaitForApp(t, page)
	AssertRootImportMap(t, harness(t), page)
	ready := WaitForDriveReady(t, harness(t), page)
	formatOptionalTiming := func(v *int) string {
		if v == nil {
			return "unset"
		}
		return strconv.Itoa(*v)
	}
	t.Logf(
		"goscript drive mount gate ready: hash=%s contentReadyMs=%d quickstartState=%s progressReadyMs=%s quickstartContentReadyMs=%s quickstartFinishedMs=%s",
		ready.Hash,
		ready.ContentReadyMs,
		ready.QuickstartState,
		formatOptionalTiming(ready.QuickstartProgressReadyMs),
		formatOptionalTiming(ready.QuickstartContentReadyMs),
		formatOptionalTiming(ready.QuickstartFinishedMs),
	)
	AssertQuickstartContentAfterProgress(t, ready)
	AssertBrowserStartupDone(t, harness(t), page)
}

// TestGoScriptSharedObjectDirectRouteBodyMountGate verifies a fresh page can
// open an existing SharedObject route through Resource SDK mounts without
// reusing quickstart handoff resources.
func TestGoScriptSharedObjectDirectRouteBodyMountGate(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve e2e wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript-only regression gate; compiler=%s", compiler)
	}

	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during direct SharedObject route body mount gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during direct SharedObject route body mount gate: %+v", report)
		}
	}()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	targetHash, err := currentHash(page.URL())
	if err != nil {
		t.Fatalf("current drive hash: %v", err)
	}
	script := "globalThis.__s4waveLogQuickstartTiming = true;"
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install quickstart timing init script: %v", err)
	}
	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page in current context: %v", err)
	}
	if err := harness(t).loadAppPageURL(sess, harness(t).BaseURL()+"/"+targetHash); err != nil {
		t.Fatalf("load direct SharedObject route after page replacement: %v", err)
	}

	page = sess.Page()
	WaitForApp(t, page)
	AssertRootImportMap(t, harness(t), page)
	AssertBrowserStartupDone(t, harness(t), page)
	WaitForDriveReady(t, harness(t), page)
	assertDirectSharedObjectRouteStartupMarks(t, page)
	assertDirectSharedObjectRouteSpaceState(t, page)

	t.Logf(
		"goscript direct SharedObject route body mount gate passed: session_index=%d space_id=%s hash=%s",
		scenario.GetSessionIndex(),
		scenario.GetSpaceID(),
		targetHash,
	)
}

// TestGoScriptQuickstartSpaceDirectRouteMountGate verifies the static Space
// quickstart can reopen its direct Space route without Quickstart handoff state.
func TestGoScriptQuickstartSpaceDirectRouteMountGate(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve e2e wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript-only regression gate; compiler=%s", compiler)
	}

	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during direct Space route gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during direct Space route gate: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, harness(t), page, "#/quickstart/space")
	WaitForEmptySpaceReady(t, page)

	targetHash, err := currentHash(page.URL())
	if err != nil {
		t.Fatalf("current Space hash: %v", err)
	}
	script := "globalThis.__s4waveLogQuickstartTiming = true;"
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install quickstart timing init script: %v", err)
	}
	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page in current context: %v", err)
	}
	if err := harness(t).loadAppPageURL(sess, harness(t).BaseURL()+"/"+targetHash); err != nil {
		t.Fatalf("load direct Space route after page replacement: %v", err)
	}

	page = sess.Page()
	WaitForApp(t, page)
	AssertRootImportMap(t, harness(t), page)
	AssertBrowserStartupDone(t, harness(t), page)
	WaitForEmptySpaceReady(t, page)
	assertDirectSpaceRouteStartupMarks(t, page)
	assertDirectSpaceRouteSpaceState(t, page)

	t.Logf("goscript direct Space route gate passed: hash=%s", targetHash)
}

// TestGoScriptFSHandleBrowserResourceOperations proves the browser Resource SDK
// can drive UnixFS handle operations through a GoScript-mounted Drive.
func TestGoScriptFSHandleBrowserResourceOperations(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve e2e wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript-only regression gate; compiler=%s", compiler)
	}

	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during FSHandle operation gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during FSHandle operation gate: %+v", report)
		}
	}()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)
	assertGoScriptFSHandleBrowserResourceOperations(t, page)
	measureGoScriptUnixFSMkdir(t, sess, page)

	t.Logf(
		"goscript FSHandle browser resource operations passed: session_index=%d space_id=%s",
		scenario.GetSessionIndex(),
		scenario.GetSpaceID(),
	)
}

func assertDirectSharedObjectRouteStartupMarks(t testing.TB, page playwright.Page) {
	t.Helper()

	assertDirectRouteStartupMarks(t, page, []string{
		"quickstart.session-mount-start",
		"quickstart.session-mount-ready",
		"quickstart.shared-object-mount-start",
		"quickstart.shared-object-mount-ready",
		"quickstart.shared-object-body-mount-start",
		"quickstart.shared-object-body-mount-ready",
		"quickstart.space-resource-created",
		"quickstart.space-world-access-ready",
		"quickstart.space-contents-mount-ready",
		"unixfs.browser-mounted",
		"unixfs.seeded-file-visible",
	})
}

func assertDirectSpaceRouteStartupMarks(t testing.TB, page playwright.Page) {
	t.Helper()

	assertDirectRouteStartupMarks(t, page, []string{
		"quickstart.session-mount-start",
		"quickstart.session-mount-ready",
		"quickstart.shared-object-mount-start",
		"quickstart.shared-object-mount-ready",
		"quickstart.shared-object-body-mount-start",
		"quickstart.shared-object-body-mount-ready",
		"quickstart.space-resource-created",
		"quickstart.space-world-access-ready",
		"quickstart.space-contents-mount-ready",
	})
}

func assertDirectRouteStartupMarks(t testing.TB, page playwright.Page, required []string) {
	t.Helper()

	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label ?? mark.name ?? '',
		detail: mark.detail ?? {},
	}))`, nil)
	if err != nil {
		t.Fatalf("read startup marks: %v", err)
	}
	marks, ok := raw.([]any)
	if !ok {
		t.Fatalf("unexpected startup marks %T: %#v", raw, raw)
	}
	labels := make([]string, 0, len(marks))
	for _, mark := range marks {
		m, ok := mark.(map[string]any)
		if !ok {
			t.Fatalf("unexpected startup mark %T: %#v", mark, mark)
		}
		labels = append(labels, stringField(m, "label"))
	}

	for _, label := range required {
		if !slices.Contains(labels, label) {
			t.Fatalf("direct route missing startup mark %q; labels=%v", label, labels)
		}
	}

	for _, label := range []string{
		"quickstart.session-handoff-used",
		"quickstart.shared-object-handoff-used",
		"quickstart.shared-object-body-handoff-used",
		"quickstart.space-handoff-used",
		"quickstart.space-world-handoff-used",
		"quickstart.space-contents-handoff-used",
	} {
		if slices.Contains(labels, label) {
			t.Fatalf("direct route used quickstart handoff mark %q; labels=%v", label, labels)
		}
	}
}

func assertGoScriptFSHandleBrowserResourceOperations(t testing.TB, page playwright.Page) {
	t.Helper()

	raw, err := page.Evaluate(`async () => {
		function streamFromText(text) {
			return new ReadableStream({
				start(controller) {
					controller.enqueue(new TextEncoder().encode(text))
					controller.close()
				},
			})
		}
		async function readText(handle, length = 0n) {
			const read = await handle.readAt(0n, length)
			return new TextDecoder().decode(read.data)
		}
		async function waitForWatchEntry(iter, name) {
			const deadline = performance.now() + 15000
			for (;;) {
				const remaining = deadline - performance.now()
				if (remaining <= 0) {
					throw new Error('watchReaddir did not observe ' + name)
				}
				const next = await Promise.race([
					iter.next(),
					new Promise((_, reject) => {
						setTimeout(() => reject(new Error('watchReaddir timed out for ' + name)), Math.min(remaining, 1000))
					}),
				])
				if (next.done) {
					throw new Error('watchReaddir ended before observing ' + name)
				}
				const names = (next.value ?? []).map((entry) => entry.name ?? '')
				if (names.includes(name)) {
					return names
				}
			}
		}
		async function nextWithTimeout(iter, label, timeoutMS = 5000) {
			return await Promise.race([
				iter.next(),
				new Promise((_, reject) => {
					setTimeout(() => reject(new Error(label + ' timed out')), timeoutMS)
				}),
			])
		}
		async function expectWatchClosed(iter) {
			const isAbort = (err) => {
				const text = String(err?.name ?? '') + ' ' + String(err?.message ?? err)
				return /abort/i.test(text)
			}
			try {
					const next = await nextWithTimeout(iter, 'watchReaddir close')
				if (!next.done) {
					throw new Error('watchReaddir yielded after abort')
				}
			} catch (err) {
				if (!isAbort(err)) throw err
			} finally {
				if (typeof iter.return === 'function') {
					try {
						await iter.return()
					} catch (err) {
						if (!isAbort(err)) throw err
					}
				}
			}
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		const FSHandle = debug?.FSHandle
		const unixfsObjectKey = debug?.UNIXFS_OBJECT_KEY
		const mknodType = debug?.MknodType ?? { FILE: 1, DIR: 2 }
		if (!match || !root || !mountSpace || !FSHandle || !unixfsObjectKey) {
			return { error: 'missing direct Drive route or debug FSHandle context' }
		}
		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		const mountedResources = {
			session: null,
			space: null,
			world: null,
			rootHandle: null,
		}
		let watchAbort = null
		let step = 'mount-session'
		try {
			const abort = AbortSignal.timeout(120000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session' }
			step = 'mount-space'
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: {
							id: sharedObjectId,
						},
					},
				},
				abortSignal: abort,
				cleanup,
			})
			step = 'access-world'
			mountedResources.world = await mountedResources.space.accessWorldState(true, abort)
			step = 'access-unixfs'
			const access = await mountedResources.world.accessTypedObject(unixfsObjectKey, abort)
			if (!access?.resourceId) return { error: 'accessTypedObject returned no UnixFS resource id' }
			mountedResources.rootHandle = new FSHandle(
				mountedResources.world.getResourceRef().createRef(access.resourceId),
			)

			const rootHandle = mountedResources.rootHandle
			step = 'read-starter-file'
			const starter = await rootHandle.lookup('getting-started.md', abort)
			const starterText = await readText(starter)
			starter.release()
			if (!starterText.includes('Getting Started')) {
				return { error: 'starter file read did not contain expected text' }
			}

			const initialEntries = (await rootHandle.readdirAll(0n, abort)).map((entry) => entry.name ?? '')
			if (!initialEntries.includes('getting-started.md')) {
				return { error: 'root readdir did not include starter file' }
			}

			step = 'watch-create-remove'
			watchAbort = new AbortController()
			const watchIter = rootHandle.watchReaddir(watchAbort.signal)[Symbol.asyncIterator]()
			await nextWithTimeout(watchIter, 'initial watchReaddir snapshot')
			await rootHandle.mknod(['row2-watch-file.txt'], mknodType.FILE, 0o644, true, abort)
			const watchedNames = await waitForWatchEntry(watchIter, 'row2-watch-file.txt')
			watchAbort.abort()
			await expectWatchClosed(watchIter)
			await rootHandle.remove(['row2-watch-file.txt'], abort)

			step = 'file-create-write-truncate'
			await rootHandle.mknod(['row2-created.txt'], mknodType.FILE, 0o644, true, abort)
			let file = await rootHandle.lookup('row2-created.txt', abort)
			await file.writeAt(0n, new TextEncoder().encode('abcdef'), abort)
			if (await readText(file) !== 'abcdef') {
				return { error: 'write/read round trip mismatch' }
			}
			await file.truncate(3n, abort)
			const truncatedText = await readText(file)
			const truncatedSize = await file.getSize(abort)
			file.release()
			if (truncatedText !== 'abc' || truncatedSize !== 3n) {
				return { error: 'truncate mismatch: ' + truncatedText + ' size=' + truncatedSize.toString() }
			}

			step = 'directory-create-read'
			await rootHandle.mkdirAll(['row2-dir', 'nested'], 0o755, abort)
			const nested = await rootHandle.lookupPath('row2-dir/nested', abort)
			const nestedEntries = await nested.handle.readdirAll(0n, abort)
			nested.handle.release()
			if (!Array.isArray(nestedEntries)) {
				return { error: 'nested directory readdir did not return entries' }
			}

			step = 'rename-remove'
			await rootHandle.rename('row2-created.txt', 'row2-renamed.txt', 0, abort)
			file = await rootHandle.lookup('row2-renamed.txt', abort)
			if (await readText(file, 3n) !== 'abc') {
				return { error: 'renamed file read mismatch' }
			}
			file.release()
			await rootHandle.remove(['row2-renamed.txt'], abort)
			const row2Dir = await rootHandle.lookup('row2-dir', abort)
			await row2Dir.remove(['nested'], abort)
			row2Dir.release()
			await rootHandle.remove(['row2-dir'], abort)

			step = 'upload-file'
			const uploadedFileBytes = await rootHandle.uploadFile(
				'row2-upload.txt',
				6n,
				streamFromText('upload'),
				0o644,
				undefined,
				abort,
			)
			const uploadedFile = await rootHandle.lookup('row2-upload.txt', abort)
			const uploadedFileText = await readText(uploadedFile)
			uploadedFile.release()
			if (uploadedFileBytes !== 6n || uploadedFileText !== 'upload') {
				return { error: 'single-file upload mismatch' }
			}

			step = 'upload-tree'
			const treeUpload = await rootHandle.uploadTree(
				[
					{ kind: 'directory', path: 'row2-tree', mode: 0o755 },
					{
						kind: 'file',
						path: 'row2-tree/leaf.txt',
						totalSize: 4n,
						stream: streamFromText('leaf'),
						mode: 0o644,
					},
					{ kind: 'directory', path: 'row2-tree/inner', mode: 0o755 },
					{
						kind: 'file',
						path: 'row2-tree/inner/nested.txt',
						totalSize: 6n,
						stream: streamFromText('nested'),
						mode: 0o644,
					},
				],
				undefined,
				abort,
			)
			const leaf = await rootHandle.lookupPath('row2-tree/leaf.txt', abort)
			const nestedUpload = await rootHandle.lookupPath('row2-tree/inner/nested.txt', abort)
			const leafText = await readText(leaf.handle)
			const nestedUploadText = await readText(nestedUpload.handle)
			leaf.handle.release()
			nestedUpload.handle.release()
			if (leafText !== 'leaf' || nestedUploadText !== 'nested') {
				return { error: 'tree upload read mismatch' }
			}

			const finalEntries = (await rootHandle.readdirAll(0n, abort)).map((entry) => entry.name ?? '').sort()
			return {
				starterRead: true,
				initialEntries,
				watchedNames,
				fileWriteRead: true,
				truncate: true,
				directoryRead: true,
				rename: true,
				remove: true,
				uploadFileBytes: uploadedFileBytes.toString(),
				treeUploadBytes: (treeUpload.bytesWritten ?? 0n).toString(),
				treeUploadFiles: (treeUpload.filesWritten ?? 0n).toString(),
				treeUploadDirectories: (treeUpload.directoriesWritten ?? 0n).toString(),
				finalEntries,
			}
		} catch (err) {
			return { error: String(err?.stack ?? err), step }
		} finally {
			watchAbort?.abort?.()
			mountedResources.rootHandle?.release?.()
			mountedResources.world?.release?.()
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("run FSHandle browser resource operation proof: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected FSHandle operation proof %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("FSHandle operation proof failed at %s: %s", stringField(result, "step"), errMsg)
	}
	for _, key := range []string{
		"starterRead",
		"fileWriteRead",
		"truncate",
		"directoryRead",
		"rename",
		"remove",
	} {
		if !boolField(result, key) {
			t.Fatalf("FSHandle operation proof missing %s: %#v", key, result)
		}
	}
	if stringField(result, "uploadFileBytes") != "6" {
		t.Fatalf("FSHandle uploadFile bytes mismatch: %#v", result)
	}
	if stringField(result, "treeUploadBytes") != "10" {
		t.Fatalf("FSHandle uploadTree bytes mismatch: %#v", result)
	}
}

func assertDirectSharedObjectRouteSpaceState(t testing.TB, page playwright.Page) {
	t.Helper()

	raw, err := page.Evaluate(`async () => {
		async function firstStreamValue(stream) {
			for await (const value of stream) {
				return value
			}
			return null
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		if (!match || !root || !mountSpace) {
			return { error: 'missing direct SharedObject route or debug root' }
		}
		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const mountedResources = {
			session: null,
			space: null,
		}
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		try {
			const abort = AbortSignal.timeout(15000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session' }
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: {
							id: sharedObjectId,
						},
					},
				},
				abortSignal: abort,
				cleanup,
			})
			const state = await firstStreamValue(mountedResources.space.watchSpaceState({}, abort))
			return {
				ready: !!state?.ready,
				indexPath: state?.settings?.indexPath ?? '',
				objectKeys: (state?.worldContents?.objects ?? []).map((obj) => obj.objectKey ?? ''),
			}
		} catch (err) {
			return { error: String(err?.stack ?? err) }
		} finally {
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read direct SharedObject route Space state: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected direct SharedObject route Space state %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("direct SharedObject route Space state probe failed: %s", errMsg)
	}
	if !boolField(result, "ready") {
		t.Fatalf("direct SharedObject route Space state was not ready: %#v", result)
	}
	objectKeys, ok := result["objectKeys"].([]any)
	if !ok || len(objectKeys) == 0 {
		t.Fatalf("direct SharedObject route Space state had no world contents: %#v", result)
	}
}

func assertDirectSpaceRouteSpaceState(t testing.TB, page playwright.Page) {
	t.Helper()

	raw, err := page.Evaluate(`async () => {
		async function firstStreamValue(stream) {
			for await (const value of stream) {
				return value
			}
			return null
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		if (!match || !root || !mountSpace) {
			return { error: 'missing direct Space route or debug root' }
		}
		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const mountedResources = {
			session: null,
			space: null,
		}
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		try {
			const abort = AbortSignal.timeout(15000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session' }
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: {
							id: sharedObjectId,
						},
					},
				},
				abortSignal: abort,
				cleanup,
			})
			const state = await firstStreamValue(mountedResources.space.watchSpaceState({}, abort))
			return {
				ready: !!state?.ready,
				indexPath: state?.settings?.indexPath ?? '',
				objectKeys: (state?.worldContents?.objects ?? []).map((obj) => obj.objectKey ?? ''),
			}
		} catch (err) {
			return { error: String(err?.stack ?? err) }
		} finally {
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read direct Space route Space state: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected direct Space route Space state %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("direct Space route Space state probe failed: %s", errMsg)
	}
	if !boolField(result, "ready") {
		t.Fatalf("direct Space route Space state was not ready: %#v", result)
	}
	if indexPath := stringField(result, "indexPath"); indexPath != "" {
		t.Fatalf("direct Space route indexPath=%q want empty: %#v", indexPath, result)
	}
}

// TestQuickstartDriveTrace writes a trace artifact for the drive quickstart
// startup flow using client-side routing without a full page reload.
func TestQuickstartDriveTrace(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	data, err := sess.CaptureTrace(ctx, "quickstart-drive", func(ctx context.Context) error {
		NavigateHash(t, harness(t), page, "#/quickstart/drive")
		WaitForDriveReady(t, harness(t), page)
		return nil
	})
	if err != nil {
		t.Fatalf("CaptureTrace: %v", err)
	}

	path := TraceArtifactPath(t)
	if err := WriteTraceArtifact(path, data); err != nil {
		t.Fatalf("WriteTraceArtifact: %v", err)
	}
	t.Logf("trace artifact written to %s (%d bytes)", path, len(data))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty trace artifact")
	}
}

// TestDriveNavigationBurstTrace writes a trace artifact for repeated
// file-open / navigate-up cycles in the drive browser. This exercises the
// block commit hot path with sustained navigation traffic, producing
// enough write transactions to measure coalescing and batching behavior.
func TestDriveNavigationBurstTrace(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	const rounds = 12
	const releasedErr = "resource or inode was released"
	const welcomeMsg = "Welcome to your new drive"

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, harness(t), page, "#/quickstart/drive")
	WaitForDriveReady(t, harness(t), page)

	browser := page.Locator("[data-testid='unixfs-browser']")
	content := browser.Locator("text=" + welcomeMsg).First()
	upBtn := page.Locator("button[title='Up']")

	data, err := sess.CaptureTrace(ctx, "drive-navigation-burst", func(ctx context.Context) error {
		for i := range rounds {
			// Open getting-started.md
			row := browser.Locator("text=getting-started.md").First()
			if err := row.WaitFor(); err != nil {
				return err
			}
			if err := row.Dblclick(); err != nil {
				return err
			}
			if err := content.WaitFor(); err != nil {
				return err
			}
			txt, err := browser.TextContent()
			if err != nil {
				return err
			}
			if !strings.Contains(txt, welcomeMsg) {
				return errors.New("expected getting-started content to render")
			}
			if strings.Contains(txt, releasedErr) {
				return errors.New("getting-started view rendered released-resource error")
			}

			// Navigate back up to the listing
			if err := upBtn.Click(); err != nil {
				return err
			}
			if err := row.WaitFor(); err != nil {
				return err
			}

			t.Logf("navigation round %s complete", strconv.Itoa(i+1))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CaptureTrace: %v", err)
	}

	path := TraceArtifactPath(t)
	if err := WriteTraceArtifact(path, data); err != nil {
		t.Fatalf("WriteTraceArtifact: %v", err)
	}
	t.Logf("trace artifact written to %s (%d bytes)", path, len(data))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty trace artifact")
	}
}

// pluginServiceID returns the plugin-prefixed service ID for the
// spacewave-core plugin worker.
func pluginServiceID(serviceID string) string {
	return "plugin/spacewave-core/" + serviceID
}

func newPeerInfoClient(s *TestSession) e2e_wasm_session.SRPCPeerInfoResourceServiceClient {
	return e2e_wasm_session.NewSRPCPeerInfoResourceServiceClientWithServiceID(
		s.BrowserClient(), pluginServiceID(e2e_wasm_session.SRPCPeerInfoResourceServiceServiceID))
}

func newQuicRwcFixtureClient(s *TestSession) e2e_wasm_session.SRPCQuicRwcFixtureResourceServiceClient {
	return e2e_wasm_session.NewSRPCQuicRwcFixtureResourceServiceClientWithServiceID(
		s.BrowserClient(), pluginServiceID(e2e_wasm_session.SRPCQuicRwcFixtureResourceServiceServiceID))
}

func newSignalRelayClient(s *TestSession) e2e_wasm_session.SRPCSignalRelayServiceClient {
	return e2e_wasm_session.NewSRPCSignalRelayServiceClientWithServiceID(
		s.BrowserClient(), pluginServiceID(e2e_wasm_session.SRPCSignalRelayServiceServiceID))
}

func newEstablishLinkClient(s *TestSession) e2e_wasm_session.SRPCEstablishLinkResourceServiceClient {
	return e2e_wasm_session.NewSRPCEstablishLinkResourceServiceClientWithServiceID(
		s.BrowserClient(), pluginServiceID(e2e_wasm_session.SRPCEstablishLinkResourceServiceServiceID))
}

func containsSpaceResource(spaces []*space.SpaceSoListEntry, spaceID string) bool {
	for _, entry := range spaces {
		ref := entry.GetEntry().GetRef().GetProviderResourceRef()
		if ref.GetId() == spaceID {
			return true
		}
	}
	return false
}

// TestQuickstartForgeRoute verifies the forge quickstart creates a space and
// redirects to the forge dashboard route.
func TestQuickstartForgeRoute(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, harness(t), page, "#/quickstart/forge")
	WaitForForgeViewer(t, page)

	url := page.URL()
	if url == "" {
		t.Fatal("page has no URL after forge quickstart routing")
	}
	if !strings.Contains(url, "#/u/") || !strings.Contains(url, "/so/") {
		t.Fatalf("expected forge quickstart URL, got %q", url)
	}
}

// TestForgeScenarioSequence verifies the forge quickstart flow as one ordered
// sequence: space creation, dashboard rendering, entity visibility, and
// resource mount accessibility from Go.
func TestForgeScenarioSequence(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	scenario := CreateForgeScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()

	t.Run("shell", func(t *testing.T) {
		if scenario.GetSession() != sess {
			t.Fatal("expected forge scenario to retain the owning session")
		}
		if scenario.GetSessionIndex() == 0 {
			t.Fatal("expected non-zero session index")
		}
		if scenario.GetSpaceID() == "" {
			t.Fatal("expected non-empty space id")
		}
	})

	t.Run("dashboard-ready", func(t *testing.T) {
		WaitForForgeReady(t, harness(t), page)
	})

	t.Run("state-ready", func(t *testing.T) {
		WaitForForgeReady(t, harness(t), page)

		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()

		sessions, err := sess.Root().ListSessions(ctx)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(sessions) == 0 {
			t.Fatal("expected sessions after forge quickstart")
		}

		s, err := sess.MountSessionByIdx(ctx, scenario.GetSessionIndex())
		if err != nil {
			t.Fatalf("MountSessionByIdx: %v", err)
		}
		defer s.Release()

		rlStream, err := s.WatchResourcesList(ctx)
		if err != nil {
			t.Fatalf("WatchResourcesList: %v", err)
		}
		resp, err := rlStream.Recv()
		if err != nil {
			t.Fatalf("WatchResourcesList recv: %v", err)
		}
		rlStream.Close()

		spaces := resp.GetSpacesList()
		if !containsSpaceResource(spaces, scenario.GetSpaceID()) {
			t.Fatalf("expected quickstart-created space %q in resources list", scenario.GetSpaceID())
		}
		t.Logf(
			"state ready: quickstart-created space %s present in %d space(s)",
			scenario.GetSpaceID(),
			len(spaces),
		)
	})

	t.Run("entity-navigation", func(t *testing.T) {
		WaitForForgeReady(t, harness(t), page)

		// Click on the first entity link in the dashboard to navigate into a viewer.
		link := page.Locator("[data-testid='forge-viewer'] a").First()
		err := link.WaitFor()
		if err != nil {
			t.Skipf("no entity links in forge dashboard, skipping navigation: %v", err)
		}
		if err := link.Click(); err != nil {
			t.Fatalf("click entity link: %v", err)
		}

		// After navigation the forge viewer shell should still be present.
		WaitForForgeViewer(t, page)
	})
}

// TestForgeWorkerExecution verifies binding approval starts the quickstart
// worker and drives the Forge pass/execution path to a completed Job.
func TestForgeWorkerExecution(t *testing.T) {
	h := harness(t)
	sess := h.NewCleanSession(t)

	scenario := CreateForgeScenario(t, h, sess)
	page := scenario.GetSession().Page()
	WaitForForgeReady(t, h, page)

	ctx := t.Context()
	mounted := mountForgeSpace(ctx, t, sess, scenario.GetSessionIndex(), scenario.GetSpaceID())
	defer mounted.Release()

	const jobKey = "sample-job"
	const workerKey = "session-worker"

	assertNoForgePasses(ctx, t, mounted.eng, jobKey)
	stopForgeObserver := observeForgeWorld(ctx, h.le, mounted.engWs, jobKey)
	defer stopForgeObserver()

	_, err := mounted.contentsSvc.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: workerKey,
		TypeId:    "forge/worker",
		Approved:  true,
	})
	if err != nil {
		t.Fatalf("SetProcessBinding: %v", err)
	}

	stateStream, err := mounted.contentsSvc.WatchState(ctx, &s4wave_space.WatchSpaceContentsStateRequest{})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}
	defer stateStream.Close()
	state, err := stateStream.Recv()
	if err != nil {
		t.Fatalf("WatchState Recv: %v", err)
	}
	if len(state.GetProcessBindings()) == 0 || !state.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("expected approved worker binding, got %+v", state.GetProcessBindings())
	}

	// Start the completion budget after setup has observed the approved worker.
	// Browser/plugin startup and World mounting must not consume wait time.
	// The alternate browser compilers run every pass an order slower, so they
	// take the same doubled budget WaitForApp gives app readiness.
	jobBudget := 3 * time.Minute
	if E2EWasmSlowCompilerEnabled() {
		jobBudget = 6 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, jobBudget)
	defer cancel()
	job, err := forge_job.WaitJobComplete(waitCtx, h.le, mounted.engWs, jobKey)
	if err != nil {
		t.Fatalf("WaitJobComplete: %v", err)
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("validate completed Job: %v", err)
	}
	if failErr := job.GetResult().GetFailError(); failErr != "" {
		t.Fatalf("completed Job failed: %s", failErr)
	}
}

// TestQuickstartForgeTrace writes a trace artifact for the forge quickstart
// startup flow.
func TestQuickstartForgeTrace(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	data, err := sess.CaptureTrace(ctx, "quickstart-forge", func(ctx context.Context) error {
		NavigateHash(t, harness(t), page, "#/quickstart/forge")
		WaitForForgeReady(t, harness(t), page)
		return nil
	})
	if err != nil {
		t.Fatalf("CaptureTrace: %v", err)
	}

	path := TraceArtifactPath(t)
	if err := WriteTraceArtifact(path, data); err != nil {
		t.Fatalf("WriteTraceArtifact: %v", err)
	}
	t.Logf("trace artifact written to %s (%d bytes)", path, len(data))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty trace artifact")
	}
}

// TestQuickstartDriveNavigateTrace writes a trace artifact for navigating from
// the drive listing into a file via the real UI double-click path.
func TestQuickstartDriveNavigateTrace(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, harness(t), page, "#/quickstart/drive")
	WaitForDriveReady(t, harness(t), page)

	data, err := sess.CaptureTrace(ctx, "quickstart-drive-navigate", func(ctx context.Context) error {
		row := page.Locator("[data-testid='unixfs-browser']").Locator("text=getting-started.md").First()
		if err := row.WaitFor(); err != nil {
			return err
		}
		if err := row.Dblclick(); err != nil {
			return err
		}
		return page.Locator("[data-testid='unixfs-browser'] pre").First().WaitFor()
	})
	if err != nil {
		t.Fatalf("CaptureTrace: %v", err)
	}

	path := TraceArtifactPath(t)
	if err := WriteTraceArtifact(path, data); err != nil {
		t.Fatalf("WriteTraceArtifact: %v", err)
	}
	t.Logf("trace artifact written to %s (%d bytes)", path, len(data))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty trace artifact")
	}
}
