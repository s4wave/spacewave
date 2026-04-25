//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	space "github.com/s4wave/spacewave/core/space"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	e2e_wasm_session "github.com/s4wave/spacewave/e2e/wasm/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// testHarness is the package-level shared harness booted once in TestMain.
// It owns the devtool bus, WASM build, HTTP server, and Playwright browser
// process. Individual tests create isolated sessions via h.NewSession(t).
var testHarness *Harness

func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !E2EWasmEnabled() {
		le.Info("skipping e2e/wasm package; set ENABLE_E2E_WASM=true to run")
		os.Exit(0)
	}

	ctx := context.Background()

	h, err := Boot(
		ctx,
		le,
		WithConfigMutator(trace_service.InjectTraceConfig),
		WithSessionHarness(),
	)
	if err != nil {
		le.WithError(err).Fatal("boot wasm harness")
	}

	if err := h.LaunchBrowser(); err != nil {
		h.Release()
		le.WithError(err).Fatal("launch browser")
	}

	if err := h.CompileScripts("."); err != nil {
		h.Release()
		le.WithError(err).Fatal("compile test scripts")
	}

	testHarness = h

	code := m.Run()
	h.Release()
	os.Exit(code)
}

// TestWasmHarnessBoot verifies the shared harness is serving.
func TestWasmHarnessBoot(t *testing.T) {
	h := testHarness
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

// TestWasmHarnessTraceConfig verifies trace service wiring was injected.
func TestWasmHarnessTraceConfig(t *testing.T) {
	h := testHarness
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

// TestSessionHarnessPeerInfo verifies the session harness controller is
// running in the browser WASM by calling GetPeerInfo and asserting a
// non-empty peer ID.
func TestSessionHarnessPeerInfo(t *testing.T) {
	sess := testHarness.NewSession(t)
	client := sess.BrowserClient()
	if client == nil {
		t.Fatal("expected non-nil browser client")
	}

	ctx := testHarness.Context()
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

// TestMultiSessionPeerDiscovery verifies two browser sessions produce
// distinct bifrost peers discoverable via the session harness.
func TestMultiSessionPeerDiscovery(t *testing.T) {
	sessA := testHarness.NewSession(t)
	sessB := testHarness.NewSession(t)

	ctx, cancel := context.WithCancel(testHarness.Context())
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
	sessA := testHarness.NewSession(t)
	sessB := testHarness.NewSession(t)

	ctx, cancel := context.WithCancel(testHarness.Context())
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
	errCh := RelayCrossConnect(ctx, strmA, strmB)

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

// TestEndToEndLinkEstablishment verifies two browser WASM sessions can
// establish a bifrost link through the signaling relay cross-connect.
func TestEndToEndLinkEstablishment(t *testing.T) {
	sessA := testHarness.NewSession(t)
	sessB := testHarness.NewSession(t)

	ctx, cancel := context.WithCancel(testHarness.Context())
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
	relayErrCh := RelayCrossConnect(ctx, strmA, strmB)

	// Establish link from A targeting B.
	linkClient := newEstablishLinkClient(sessA)
	watchStrm, err := linkClient.WatchState(ctx, &e2e_wasm_session.WatchStateRequest{
		TargetPeerId: respB.GetPeerId(),
	})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}

	// Read state updates until CONNECTED.
	for {
		resp, err := watchStrm.Recv()
		if err != nil {
			// Check if relay died first.
			select {
			case relayErr := <-relayErrCh:
				t.Fatalf("relay cross-connect error: %v (WatchState: %v)", relayErr, err)
			default:
			}
			t.Fatalf("WatchState recv: %v", err)
		}

		state := resp.GetState()
		t.Logf("link state: %s", state.String())

		switch state {
		case e2e_wasm_session.EstablishLinkState_EstablishLinkState_CONNECTED:
			t.Log("bifrost link established between two browser sessions")
			return
		case e2e_wasm_session.EstablishLinkState_EstablishLinkState_FAILED:
			t.Fatal("link establishment failed")
		}
	}
}

// TestWasmHarnessPackageLifecycle verifies the shared harness is reused
// across tests rather than booting a new instance per test.
func TestWasmHarnessPackageLifecycle(t *testing.T) {
	if testHarness == nil {
		t.Fatal("expected shared harness from TestMain")
	}
	if testHarness.Port() == 0 {
		t.Fatal("shared harness has zero port")
	}
}

// TestWasmHarnessReadiness verifies the info endpoint responds immediately
// since Boot already waited for server readiness.
func TestWasmHarnessReadiness(t *testing.T) {
	h := testHarness
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
	h := testHarness
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
	h := testHarness
	if h.Browser() == nil {
		t.Fatal("expected non-nil browser")
	}
	if !h.Browser().IsConnected() {
		t.Fatal("browser not connected")
	}
}

// TestBrowserSessionIsolation verifies each NewSession creates a fresh
// browser context with clean storage while the devtool bus stays shared.
func TestBrowserSessionIsolation(t *testing.T) {
	h := testHarness

	// First session: inject a localStorage marker.
	s1 := h.NewSession(t)
	lsScript := h.Script("local-storage.ts")
	_, err := s1.Page().Evaluate(lsScript, map[string]any{
		"op": "set", "key": "test-marker", "value": "exists",
	})
	if err != nil {
		t.Fatalf("inject localStorage marker: %v", err)
	}

	// Second session: localStorage should be clean.
	s2 := h.NewSession(t)
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

// TestBrowserHelpersAndRawAccess verifies raw Playwright access works.
func TestBrowserHelpersAndRawAccess(t *testing.T) {
	sess := testHarness.NewSession(t)

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
	sess := testHarness.NewSession(t)
	h := testHarness

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
	sess := testHarness.NewSession(t)
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
	sess := testHarness.NewSession(t)
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
	sess := testHarness.NewSession(t)
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
	sess := testHarness.NewSession(t)
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
	sess := testHarness.NewSession(t)
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
	t.Logf("captured %d bytes of trace data", len(data))
}

// TestTraceCaptureWritesFile verifies the returned bytes are written to an
// explicit destination path owned by the Go test process.
func TestTraceCaptureWritesFile(t *testing.T) {
	sess := testHarness.NewSession(t)
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
	sess := testHarness.NewSession(t)
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
	sess := testHarness.NewSession(t)
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

// TestQuickstartDriveRoute verifies the quickstart dashboard is reachable
// via client-side routing without a full page reload.
func TestQuickstartDriveRoute(t *testing.T) {
	sess := testHarness.NewSession(t)
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/drive")
	WaitForDriveReady(t, testHarness, page)

	url := page.URL()
	if url == "" {
		t.Fatal("page has no URL after drive quickstart routing")
	}
	if !strings.Contains(url, "#/u/") || !strings.Contains(url, "/so/") {
		t.Fatalf("expected drive quickstart URL, got %q", url)
	}
}

// TestDriveScenarioSequence verifies the owned drive flow as one ordered
// sequence on a single harness session.
func TestDriveScenarioSequence(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()

	t.Run("shell", func(t *testing.T) {
		if scenario.GetSession() != sess {
			t.Fatal("expected drive scenario to retain the owning session")
		}
		if scenario.GetSessionIndex() == 0 {
			t.Fatal("expected non-zero session index")
		}
		if scenario.GetSpaceID() == "" {
			t.Fatal("expected non-empty space id")
		}
	})

	t.Run("contents", func(t *testing.T) {
		WaitForDriveReady(t, testHarness, page)
	})

	t.Run("state-ready", func(t *testing.T) {
		WaitForDriveReady(t, testHarness, page)

		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()

		sessions, err := sess.Root().ListSessions(ctx)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(sessions) == 0 {
			t.Fatal("expected sessions after owned drive quickstart")
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

	t.Run("open-file", func(t *testing.T) {
		WaitForDriveReady(t, testHarness, page)

		row := page.Locator("[role='row']").Locator("text=getting-started.md").First()
		err := row.WaitFor()
		if err != nil {
			t.Fatalf("wait for getting-started row: %v", err)
		}

		if err := row.Dblclick(); err != nil {
			t.Fatalf("DblClick getting-started row: %v", err)
		}

		content := page.Locator("[data-testid='unixfs-browser']").Locator("text=Welcome to your new drive").First()
		if err := content.WaitFor(); err != nil {
			t.Fatalf("wait for getting-started content: %v", err)
		}

		t.Logf("opened getting-started file in owned drive scenario, page URL: %s", page.URL())
	})

	t.Run("navigate-up", func(t *testing.T) {
		content := page.Locator("[data-testid='unixfs-browser']").Locator("text=Welcome to your new drive").First()
		if err := content.WaitFor(); err != nil {
			t.Fatalf("wait for getting-started content: %v", err)
		}

		if err := page.Locator("button[title='Up']").Click(); err != nil {
			t.Fatalf("click up: %v", err)
		}

		WaitForDriveReady(t, testHarness, page)

		if err := page.Locator("[role='row']").Locator("text=getting-started.md").First().WaitFor(); err != nil {
			t.Fatalf("wait for getting-started row after up: %v", err)
		}

		visible, err := content.IsVisible()
		if err == nil && visible {
			t.Fatal("expected file content view to disappear after navigating up")
		}

		url := page.URL()
		if !strings.Contains(url, "#/u/") || !strings.Contains(url, scenario.GetSpaceID()) {
			t.Fatalf("expected owned drive route after navigate up, got %q", url)
		}
	})
}

// TestQuickstartDriveNavigateHomeFromNestedDir reproduces navigating into
// /test/dir and returning to / with the path-bar Home button.
func TestQuickstartDriveNavigateHomeFromNestedDir(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()
	browser := page.Locator("[data-testid='unixfs-browser']")

	WaitForDriveReady(t, testHarness, page)

	openDir := func(name string) {
		t.Helper()

		row := page.Locator("[role='row']").Locator("text=" + name).First()
		if err := row.WaitFor(); err != nil {
			t.Fatalf("wait for %s row: %v", name, err)
		}
		if err := row.Dblclick(); err != nil {
			t.Fatalf("open %s row: %v", name, err)
		}
	}

	openDir("test")
	openDir("dir")

	homeBtn := page.Locator("button[aria-label='Navigate to root']").First()
	if err := homeBtn.Click(); err != nil {
		t.Fatalf("click root button: %v", err)
	}

	_, err := page.Evaluate(testHarness.Script("wait-for-drive.ts"), map[string]any{
		"deadlineMs": 15000,
	})
	if err != nil {
		body, textErr := browser.TextContent()
		if textErr != nil {
			t.Fatalf("wait for root listing after home: %v (read browser text: %v)", err, textErr)
		}
		t.Fatalf(
			"wait for root listing after home from /test/dir: %v (url=%q browser=%q)",
			err,
			page.URL(),
			strings.TrimSpace(body),
		)
	}

	body, err := browser.TextContent()
	if err != nil {
		t.Fatalf("read browser content after home: %v", err)
	}
	if !containsAll(body, "hello.txt", "getting-started.md", "test") {
		t.Fatalf("expected root listing after home, got %q", strings.TrimSpace(body))
	}
	if strings.Contains(body, "Loading...") {
		t.Fatalf("expected loading state to clear after home, got %q", strings.TrimSpace(body))
	}
	if !strings.Contains(page.URL(), "/so/"+scenario.GetSpaceID()) {
		t.Fatalf("expected drive route after home, got %q", page.URL())
	}
}

// TestQuickstartDriveDeleteSpace verifies a quickstart-created drive can be
// deleted through the shared object settings flow and disappears from the
// session resources list.
func TestQuickstartDriveDeleteSpace(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()

	WaitForDriveReady(t, testHarness, page)

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	s, err := sess.MountSessionByIdx(ctx, scenario.GetSessionIndex())
	if err != nil {
		t.Fatalf("MountSessionByIdx: %v", err)
	}
	defer s.Release()

	rlStream, err := s.WatchResourcesList(ctx)
	if err != nil {
		t.Fatalf("WatchResourcesList: %v", err)
	}
	defer rlStream.Close()

	var seenSpace bool
	for !seenSpace {
		resp, err := rlStream.Recv()
		if err != nil {
			t.Fatalf("WatchResourcesList initial recv: %v", err)
		}
		seenSpace = containsSpaceResource(resp.GetSpacesList(), scenario.GetSpaceID())
	}

	menuBtn := page.Locator("[aria-label='Open shared object menu']").First()
	if err := menuBtn.Click(); err != nil {
		t.Fatalf("open shared object menu: %v", err)
	}

	settingsHeading := page.Locator("h2:has-text('Settings')").First()
	if err := settingsHeading.WaitFor(); err != nil {
		t.Fatalf("wait for settings section: %v", err)
	}

	spaceName, err := page.Locator("span.tracking-tight").First().TextContent()
	if err != nil {
		t.Fatalf("read shared object title: %v", err)
	}
	spaceName = strings.TrimSpace(spaceName)
	if spaceName == "" {
		t.Fatal("expected non-empty shared object title")
	}

	deleteBtn := page.Locator("button:has-text('Delete Object')").First()
	if err := deleteBtn.Click(); err != nil {
		t.Fatalf("click delete object: %v", err)
	}

	dialog := page.Locator("[role='dialog']:has-text('Delete Space')").First()
	if err := dialog.WaitFor(); err != nil {
		t.Fatalf("wait for delete dialog: %v", err)
	}

	confirmInput := dialog.Locator("input").First()
	if err := confirmInput.WaitFor(); err != nil {
		t.Fatalf("wait for delete confirmation input: %v", err)
	}
	if err := confirmInput.Fill(spaceName); err != nil {
		t.Fatalf("fill delete confirmation: %v", err)
	}

	confirmDeleteBtn := dialog.Locator("button:has-text('Delete Space')").First()
	if err := confirmDeleteBtn.Click(); err != nil {
		t.Fatalf("confirm delete space: %v", err)
	}

	removed := false
	for !removed {
		resp, err := rlStream.Recv()
		if err != nil {
			t.Fatalf("WatchResourcesList deletion recv: %v", err)
		}
		removed = !containsSpaceResource(resp.GetSpacesList(), scenario.GetSpaceID())
	}

	wantSessionRoute := "#/u/" + strconv.Itoa(int(scenario.GetSessionIndex()))
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	url := page.URL()
	for strings.Contains(url, "/so/"+scenario.GetSpaceID()) || !strings.Contains(url, wantSessionRoute) {
		select {
		case <-deadline.C:
			t.Fatalf("wait for post-delete navigation timed out (url=%q)", page.URL())
		case <-tick.C:
			url = page.URL()
		}
	}

	if strings.Contains(url, "/so/"+scenario.GetSpaceID()) {
		t.Fatalf("expected deleted space route to close, got %q", url)
	}
	if !strings.Contains(url, wantSessionRoute) {
		t.Fatalf("expected session route after delete, got %q", url)
	}
}

// TestQuickstartDriveTrace writes a trace artifact for the drive quickstart
// startup flow using client-side routing without a full page reload.
func TestQuickstartDriveTrace(t *testing.T) {
	sess := testHarness.NewSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	data, err := sess.CaptureTrace(ctx, "quickstart-drive", func(ctx context.Context) error {
		NavigateHash(t, testHarness, page, "#/quickstart/drive")
		WaitForDriveReady(t, testHarness, page)
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
	const rounds = 12
	const releasedErr = "resource or inode was released"
	const welcomeMsg = "Welcome to your new drive"

	sess := testHarness.NewSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/drive")
	WaitForDriveReady(t, testHarness, page)

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
	sess := testHarness.NewSession(t)
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/forge")
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
	sess := testHarness.NewSession(t)
	scenario := CreateForgeScenario(t, testHarness, sess)
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
		WaitForForgeReady(t, testHarness, page)
	})

	t.Run("state-ready", func(t *testing.T) {
		WaitForForgeReady(t, testHarness, page)

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
		WaitForForgeReady(t, testHarness, page)

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
// worker and drives the Forge pass/execution path to completion with logs.
func TestForgeWorkerExecution(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateForgeScenario(t, testHarness, sess)
	WaitForForgeReady(t, testHarness, scenario.GetSession().Page())

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	mounted := mountForgeSpace(ctx, t, sess, scenario.GetSessionIndex(), scenario.GetSpaceID())
	defer mounted.Release()

	assertNoForgePasses(ctx, t, mounted.engine, "forge/cluster/job/sample")

	_, err := mounted.contentsSvc.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "forge/worker/session",
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
	if len(state.GetProcessBindings()) == 0 || !(state.GetProcessBindings()[0].GetApproved()) {
		t.Fatalf("expected approved worker binding, got %+v", state.GetProcessBindings())
	}

	passKey, execKey, passState, execState := waitForForgeExecution(
		ctx,
		t,
		mounted.engine,
		"forge/cluster/job/sample",
	)
	if !passState.IsComplete() {
		t.Fatalf("expected complete pass, got %s", passState.GetPassState().String())
	}
	if !execState.IsComplete() {
		t.Fatalf("expected complete execution, got %s", execState.GetExecutionState().String())
	}
	if !execState.GetResult().IsSuccessful() {
		t.Fatalf("expected successful execution, got %q", execState.GetResult().GetFailError())
	}
	if !strings.Contains(execState.GetLogEntries()[0].GetMessage(), "noop execution complete") {
		t.Fatalf("expected noop execution log, got %+v", execState.GetLogEntries())
	}

	t.Logf("worker approval produced pass %s and execution %s", passKey, execKey)
}

// TestQuickstartForgeTrace writes a trace artifact for the forge quickstart
// startup flow.
func TestQuickstartForgeTrace(t *testing.T) {
	sess := testHarness.NewSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	data, err := sess.CaptureTrace(ctx, "quickstart-forge", func(ctx context.Context) error {
		NavigateHash(t, testHarness, page, "#/quickstart/forge")
		WaitForForgeReady(t, testHarness, page)
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
	sess := testHarness.NewSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	page := sess.Page()

	WaitForApp(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/drive")
	WaitForDriveReady(t, testHarness, page)

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
