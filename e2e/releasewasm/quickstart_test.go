//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestQuickstartPrerenderAutoBootsProductionWasmBundle(t *testing.T) {
	// Read the release descriptor for the quickstart smoke artifact.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Remove the previous quickstart smoke artifact.
	path := testHarness.quickstartSmokeArtifactPath(t)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove previous quickstart smoke artifact: %v", err)
	}

	// Open a traced Drive quickstart page with its source revision.
	source := sourceRevision(t)
	page := testHarness.newPage(t)
	traceCapture := beginQuickstartRuntimeTrace(t, page)
	defer traceCapture.cleanup(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}
	enableQuickstartTimingLogs(t, page)

	// Wait for the prerendered Drive quickstart to mount its frame.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	waitForQuickstartAppRoute(t, page)
	completeQuickstartDriveIntroIfPresent(t, page)
	err = waitForDriveFrameReady(page, browserWaitMS)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart frame-ready: %v", err)
	}

	// Require Drive content and the golden path. The failure is recorded
	// without stopping, so the smoke artifact still captures the state.
	driveFrameReadyMs := browserNowMs(t, page)
	driveContentReadyMs, driveContentReadyError := waitForQuickstartDriveContentReady(t, page)
	if driveContentReadyError != "" {
		t.Errorf("quickstart content-ready not reached: %s", driveContentReadyError)
	}
	driveGoldenPathReadyMs, driveGoldenPathError := exerciseQuickstartDriveGoldenPath(t, page)
	if driveGoldenPathError != "" {
		t.Errorf("quickstart golden path not reached: %s", driveGoldenPathError)
	}

	// Measure the post-load workload and foreground resume, then stop tracing.
	postLoadSOWorkload := runQuickstartPostLoadSOWorkload(t, page, driveContentReadyMs != nil)
	foregroundResume := collectForegroundResumeEvidence(t, page)
	logQuickstartTiming(t, page)
	runtimeTrace := traceCapture.stop(t)

	// Collect the quickstart smoke artifact from the measured browser state.
	data, err := collectQuickstartSmokeArtifact(page, desc, source, driveFrameReadyMs, driveContentReadyMs, driveContentReadyError, driveGoldenPathReadyMs, driveGoldenPathError, runtimeTrace, postLoadSOWorkload, foregroundResume)
	if err != nil {
		t.Fatalf("collect quickstart smoke artifact: %v", err)
	}

	// Preserve and report the quickstart smoke artifact.
	if err := writeQuickstartSmokeArtifact(path, data); err != nil {
		t.Fatalf("write quickstart smoke artifact: %v", err)
	}
	t.Logf("quickstart smoke artifact written to %s (%d bytes)", path, len(data))
}

func TestQuickstartSecondTabReusesRuntimeAndCloseKeepsFirstTab(t *testing.T) {
	// Open the first Drive quickstart page.
	pageA := testHarness.newPage(t)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	if _, err := pageA.Goto(quickstartURL); err != nil {
		t.Fatalf("goto first quickstart drive: %v", err)
	}

	// Wait for the first Drive quickstart frame.
	waitForQuickstartDriveFrame(t, pageA, false, "wait for first quickstart frame-ready: %v")

	// Mark the first page to detect reloads and cross-tab navigation.
	firstURL := pageA.URL()
	if _, err := pageA.Evaluate(`() => {
		const navEvents = []
		globalThis.__swCrossTabNavEvents = navEvents
		const record = (type, detail = {}) => {
			navEvents.push({
				type,
				href: location.href,
				hash: location.hash,
				time: performance.now(),
				...detail,
			})
		}
		window.addEventListener('hashchange', () => record('hashchange'))
		window.addEventListener('storage', (ev) => record('storage', {
			key: ev.key,
			newValue: ev.newValue,
		}))
		globalThis.__swCrossTabReloadProbe = {
			token: crypto.randomUUID(),
			href: location.href,
			markedAt: performance.now(),
		}
	}`); err != nil {
		t.Fatalf("install first tab reload probe: %v", err)
	}

	// Open a second Drive page in the retained browser context.
	pageB := testHarness.newPageInContext(t, pageA.Context())
	if _, err := pageB.Goto(quickstartURL); err != nil {
		t.Fatalf("goto second quickstart drive: %v", err)
	}

	// Wait for the second Drive quickstart frame.
	waitForQuickstartDriveFrame(t, pageB, true, "wait for second quickstart frame-ready: %v")

	// Close the second Drive page and foreground the first.
	if err := pageB.Close(); err != nil {
		t.Fatalf("close second quickstart tab: %v", err)
	}
	if err := pageA.BringToFront(); err != nil {
		t.Fatalf("bring first quickstart tab to front: %v", err)
	}

	// Inspect the first page after the second page closes.
	raw, err := pageA.Evaluate(`async () => {
		await new Promise((resolve) => requestAnimationFrame(() => {
			requestAnimationFrame(resolve)
		}))
		const marker = globalThis.__swCrossTabReloadProbe ?? null
		const driveReady = !!document.querySelector("[data-testid='unixfs-browser']")
		return {
			href: location.href,
			markerPresent: !!marker,
			markerHref: marker?.href ?? '',
			driveReady,
			bootStatus: globalThis.__swBootStatus ?? null,
			resumeReady: globalThis.__swWebDocumentResumeReady ?? null,
			navEvents: globalThis.__swCrossTabNavEvents ?? [],
			localTabs: localStorage.getItem('shell-tabs-state'),
			sessionTabs: sessionStorage.getItem('shell-tabs-state'),
		}
	}`)
	if err != nil {
		t.Fatalf("read first tab after closing second: %v", err)
	}

	// Decode the surviving page's state.
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected first tab close state %T", raw)
	}

	// Verify the first page retains its URL, document and Drive frame.
	if state["href"] != firstURL {
		t.Fatalf("first tab URL changed after closing second tab: got %v want %s state=%#v", state["href"], firstURL, state)
	}
	if state["markerPresent"] != true {
		dumpPageState(t, pageA)
		t.Fatalf("first tab reloaded after closing second tab: %#v", state)
	}
	if state["driveReady"] != true {
		dumpPageState(t, pageA)
		t.Fatalf("first tab lost drive readiness after closing second tab: %#v", state)
	}
}

func TestGoScriptQuickstartReturnVisitorMountsBodyRoute(t *testing.T) {
	// Require GoScript for the return visitor route proof.
	if compiler, err := resolveReleaseWasmCompiler(); err != nil {
		t.Fatalf("resolve release wasm compiler: %v", err)
	} else if compiler != releaseWasmCompilerGoScript {
		t.Skipf("release-WASM return visitor body route gate requires %s=true", E2EReleaseWasmGoScriptEnv)
	}

	// Open the first Drive quickstart page.
	pageA := testHarness.newPage(t)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	if _, err := pageA.Goto(quickstartURL); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}

	// Wait for the initial Drive frame and its created route.
	waitForQuickstartDriveFrame(t, pageA, false, "wait for quickstart frame-ready: %v")

	// Read and validate the direct SharedObject route.
	hash, err := pageA.Evaluate(`() => window.location.hash`)
	if err != nil {
		t.Fatalf("read created direct route hash: %v", err)
	}
	hashText, ok := hash.(string)
	if !ok || !strings.HasPrefix(hashText, "#/u/") || !strings.Contains(hashText, "/so/") {
		t.Fatalf("quickstart did not produce direct SharedObject route hash: %#v", hash)
	}
	directURL := testHarness.getBaseURL() + "/" + hashText

	// Open the saved SharedObject route in a second document.
	pageB := testHarness.newPageInContext(t, pageA.Context())
	if _, err := pageB.Goto(directURL); err != nil {
		t.Fatalf("goto direct SharedObject route: %v", err)
	}

	// Wait for the return visitor's Drive frame.
	waitForPrerenderRootOrLiveApp(t, pageB)
	waitForBootFunction(t, pageB)
	waitForLiveApp(t, pageB)
	waitForQuickstartAppRoute(t, pageB)
	if err := waitForDriveFrameReady(pageB, browserWaitMS); err != nil {
		dumpPageState(t, pageB)
		t.Fatalf("wait for return visitor direct route frame-ready: %v", err)
	}

	// Verify the return visitor mounts persisted content through the body route.
	if _, err := waitForQuickstartDriveContentReady(t, pageB); err != "" {
		t.Fatalf("wait for return visitor direct route content-ready: %s", err)
	}
	assertReturnVisitorBodyRouteStartupMarks(t, pageB)
	assertReturnVisitorBodyRouteSpaceState(t, pageB)
}
