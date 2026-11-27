//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

var testHarness *harness

// TIER: nightly
func TestMain(m *testing.M) {
	// Configure the release harness logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Require the release harness opt-in before starting browsers.
	if !E2EReleaseWasmEnabled() {
		le.Info("skipping e2e/releasewasm package; set ENABLE_E2E_RELEASE_WASM=true to run")
		os.Exit(0)
	}

	// Apply the release startup trace configuration.
	if err := applyReleaseStartupTraceEnv(); err != nil {
		le.WithError(err).Fatal("apply release wasm startup trace env")
	}

	// Bound the release harness lifetime and release its context.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Boot the release harness and expose it to the tests.
	h, err := boot(ctx, le)
	if err != nil {
		le.WithError(err).Fatal("boot release wasm harness")
	}
	testHarness = h

	// Run the release tests and release their browser harness.
	code := m.Run()
	h.release(le)
	os.Exit(code)
}

func TestLaunchPostPrerenderAssets(t *testing.T) {
	// Open the prerendered launch post.
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/blog/2026/04/launch"); err != nil {
		t.Fatalf("goto launch post: %v", err)
	}

	// Inspect the launch post styles and image delivery.
	raw, err := page.Evaluate(`async () => {
		const heading = document.querySelector('.blog-prose h2')
		const list = document.querySelector('.blog-prose ul')
		const paragraph = document.querySelector('.blog-prose p')
		const avatar = document.querySelector('img[alt="Christian Stewart"]')
		const signoff = [...document.querySelectorAll('.blog-prose p')].find(
			(element) => element.textContent?.includes('Thanks for checking out Spacewave!'),
		)
		if (!heading || !list || !paragraph || !avatar || !signoff) {
			throw new Error('launch post contract elements are missing')
		}
		if (!avatar.complete) {
			await new Promise((resolve) => {
				avatar.addEventListener('load', resolve, { once: true })
				avatar.addEventListener('error', resolve, { once: true })
			})
		}

		const headingStyle = getComputedStyle(heading)
		const listStyle = getComputedStyle(list)
		const paragraphStyle = getComputedStyle(paragraph)
		return {
			linkedHydrateCss: [...document.querySelectorAll('link[rel="stylesheet"]')].some(
				(link) => new URL(link.href).pathname.startsWith('/static/assets/hydrate-'),
			),
			headingFontSize: headingStyle.fontSize,
			headingFontWeight: headingStyle.fontWeight,
			headingMarginTop: headingStyle.marginTop,
			headingMarginBottom: headingStyle.marginBottom,
			listStyleType: listStyle.listStyleType,
			listPaddingLeft: listStyle.paddingLeft,
			paragraphLineHeight: paragraphStyle.lineHeight,
			paragraphMarginBottom: paragraphStyle.marginBottom,
			avatarLoaded: avatar.complete && avatar.naturalWidth > 0 && avatar.naturalHeight > 0,
			avatarSameOrigin: new URL(avatar.currentSrc || avatar.src).origin === location.origin,
			signoffHasHardBreaks: signoff.querySelectorAll('br').length === 2,
		}
	}`)
	if err != nil {
		t.Fatalf("inspect launch post: %v", err)
	}

	// Decode the launch post inspection result.
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected launch post state %T: %#v", raw, raw)
	}

	// Verify the launch post assets and signoff structure.
	for _, key := range []string{"linkedHydrateCss", "avatarLoaded", "avatarSameOrigin", "signoffHasHardBreaks"} {
		if !releaseBoolField(state, key) {
			t.Errorf("expected %s: %#v", key, state)
		}
	}

	// Verify the launch post typography and spacing.
	for key, want := range map[string]string{
		"headingFontSize":       "24px",
		"headingFontWeight":     "600",
		"headingMarginTop":      "32px",
		"headingMarginBottom":   "12px",
		"listStyleType":         "disc",
		"listPaddingLeft":       "24px",
		"paragraphLineHeight":   "28px",
		"paragraphMarginBottom": "20px",
	} {
		if got := releaseStringField(state, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestRootPrerenderLoadsProductionWasmBundle(t *testing.T) {
	// Open the prerendered release root.
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production bundle from the prerendered root.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err := page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)
}

func TestGoScriptDedicatedWorkerLocalBundleSmoke(t *testing.T) {
	// Require the GoScript compiler for the dedicated-worker smoke test.
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript dedicated-worker release smoke", E2EReleaseWasmGoScriptEnv)
	}

	// Open the release root with a dedicated worker.
	page := testHarness.newDedicatedWorkerPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production bundle and verify its dedicated-worker mode.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}
	waitForLiveApp(t, page)
	assertRuntimeWorkerMode(t, page, "dedicated-worker")
}

func TestProductionRuntimeMatchesReleaseDescriptor(t *testing.T) {
	// Read the descriptor for the runtime identity comparison.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Open the release root for the runtime comparison.
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production runtime from its prerendered root.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)

	// Read the running generation and ServiceWorker identities.
	raw, err := page.Evaluate(`async () => {
		const registration = await navigator.serviceWorker.ready
		return {
			generationId: globalThis.__swGenerationId || '',
			controllerURL: navigator.serviceWorker.controller?.scriptURL || '',
			activeURL: registration.active?.scriptURL || '',
		}
	}`)
	if err != nil {
		t.Fatalf("read production runtime state: %v", err)
	}

	// Decode the runtime identity probe.
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected production runtime state %T", raw)
	}

	// Verify the running generation matches the release descriptor.
	generationID, _ := state["generationId"].(string)
	if generationID != desc.GenerationID {
		t.Fatalf("generation id=%q want %q", generationID, desc.GenerationID)
	}

	// Verify the controlling and active ServiceWorkers match the release descriptor.
	controllerURL, _ := state["controllerURL"].(string)
	if !strings.HasSuffix(controllerURL, "/"+desc.ShellAssets.ServiceWorker) {
		t.Fatalf("controller service worker=%q want suffix %q", controllerURL, desc.ShellAssets.ServiceWorker)
	}
	activeURL, _ := state["activeURL"].(string)
	if !strings.HasSuffix(activeURL, "/"+desc.ShellAssets.ServiceWorker) {
		t.Fatalf("active service worker=%q want suffix %q", activeURL, desc.ShellAssets.ServiceWorker)
	}
}
