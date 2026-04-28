//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
	"github.com/sirupsen/logrus"
)

var testHarness *harness

const browserWaitMS = 30000

func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !E2EReleaseWasmEnabled() {
		le.Info("skipping e2e/releasewasm package; set ENABLE_E2E_RELEASE_WASM=true to run")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	h, err := boot(ctx, le)
	if err != nil {
		le.WithError(err).Fatal("boot release wasm harness")
	}
	testHarness = h

	code := m.Run()
	h.release(le)
	os.Exit(code)
}

func TestBrowserReleaseDescriptorIncludesPrerenderedWasmShell(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if desc.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", desc.SchemaVersion)
	}
	if desc.GenerationID == "" {
		t.Fatal("expected generation id")
	}
	if desc.ShellAssets.Entrypoint == "" {
		t.Fatal("expected shellAssets.entrypoint")
	}
	if desc.ShellAssets.ServiceWorker == "" {
		t.Fatal("expected shellAssets.serviceWorker")
	}
	if desc.ShellAssets.SharedWorker == "" {
		t.Fatal("expected shellAssets.sharedWorker")
	}
	if desc.ShellAssets.Wasm == "" {
		t.Fatal("expected shellAssets.wasm")
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/") {
		t.Fatalf("expected / in prerendered routes: %v", desc.PrerenderedRoutes)
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/quickstart/drive") {
		t.Fatalf("expected /quickstart/drive in prerendered routes: %v", desc.PrerenderedRoutes)
	}
}

func TestRootPrerenderLoadsProductionWasmBundle(t *testing.T) {
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

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

func TestQuickstartPrerenderAutoBootsProductionWasmBundle(t *testing.T) {
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	_, err := page.Evaluate(`async () => {
		if (window.location.hash !== '#/quickstart/drive') {
			throw new Error('expected quickstart hash, got ' + window.location.hash)
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("auto boot quickstart production wasm: %v", err)
	}
	err = page.Locator("[data-testid='unixfs-browser']").WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart drive shell: %v", err)
	}
}

func waitForLiveApp(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		await Promise.race([
			globalThis.__swReady,
			new Promise((_, reject) => setTimeout(() => reject(new Error('runtime did not become ready')), 30000)),
		])
		const deadline = performance.now() + 30000
		while (document.querySelector('#bldr-root')?.hasAttribute('data-prerendered')) {
			if (performance.now() > deadline) {
				throw new Error('prerender did not switch to live app')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for live app: %v", err)
	}
}

func waitForPrerenderRoot(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (!document.querySelector('#bldr-root[data-prerendered]')) {
			if (performance.now() > deadline) {
				throw new Error('missing prerendered bldr root')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for prerender root: %v", err)
	}
}

func waitForBootFunction(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (typeof globalThis.__swBoot !== 'function' || !globalThis.__swReady) {
			if (performance.now() > deadline) {
				throw new Error('production boot function did not initialize')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for boot function: %v", err)
	}
}

func dumpPageState(t *testing.T, page playwright.Page) {
	t.Helper()

	state, err := page.Evaluate(`() => ({
		href: window.location.href,
		title: document.title,
		text: document.body?.innerText?.slice(0, 4000) ?? '',
		rootHtml: document.querySelector('#bldr-root')?.outerHTML?.slice(0, 4000) ?? '',
	})`)
	if err != nil {
		t.Logf("dump page state: %v", err)
		return
	}
	t.Logf("page state: %#v", state)
}
