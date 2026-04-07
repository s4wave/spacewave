package comms

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/playwright-community/playwright-go"
)

var (
	testServer  *testServerState
	pwInstance  *playwright.Playwright
	distDir    string
)

type testServerState struct {
	url string
	close func()
}

// TestMain builds the fixture bundles, starts playwright, and starts the
// HTTP test server. All tests share the same server and playwright instance.
func TestMain(m *testing.M) {
	// Find the repo root (e2e/comms/ is two levels deep).
	_, thisFile, _, _ := runtime.Caller(0)
	commsDir := filepath.Dir(thisFile)
	repoRoot := filepath.Join(commsDir, "..", "..")
	distDir = filepath.Join(commsDir, "dist")

	// Build fixtures with Vite.
	fmt.Println("=== Building test fixtures with Vite ===")
	buildCmd := exec.Command("bun", "run", "vite", "build", "--config", filepath.Join(commsDir, "vite.config.ts"))
	buildCmd.Dir = repoRoot
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "vite build failed: %v\n", err)
		os.Exit(1)
	}

	// Verify dist directory has output.
	if _, err := os.Stat(filepath.Join(distDir, "detect.js")); err != nil {
		fmt.Fprintf(os.Stderr, "detect.js not found in dist: %v\n", err)
		os.Exit(1)
	}

	// Install playwright browsers if needed.
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium", "firefox", "webkit"},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "playwright install failed: %v\n", err)
		os.Exit(1)
	}

	// Start playwright.
	pw, err := playwright.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "playwright.Run failed: %v\n", err)
		os.Exit(1)
	}
	pwInstance = pw

	// Start test server.
	srv, err := newTestServer(distDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test server failed: %v\n", err)
		os.Exit(1)
	}
	testServer = &testServerState{
		url:   srv.URL,
		close: srv.Close,
	}
	fmt.Printf("=== Test server at %s ===\n", testServer.url)

	code := m.Run()

	testServer.close()
	pwInstance.Stop()
	os.Exit(code)
}

// browserType returns the playwright browser type by name.
func browserType(name string) playwright.BrowserType {
	switch name {
	case "chromium":
		return pwInstance.Chromium
	case "firefox":
		return pwInstance.Firefox
	case "webkit":
		return pwInstance.WebKit
	default:
		panic("unknown browser: " + name)
	}
}

// runFixture opens a fixture page in the given browser, waits for "DONE" in
// #log, and returns window.__results as a map.
func runFixture(t *testing.T, browserName, fixture string) map[string]interface{} {
	t.Helper()

	bt := browserType(browserName)
	browser, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		t.Fatalf("launch %s: %v", browserName, err)
	}
	defer browser.Close()

	ctx, err := browser.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	// Forward console messages to test log.
	page.On("console", func(msg playwright.ConsoleMessage) {
		t.Logf("[%s console.%s] %s", browserName, msg.Type(), msg.Text())
	})
	page.On("pageerror", func(err error) {
		t.Logf("[%s pageerror] %s", browserName, err.Error())
	})

	url := fmt.Sprintf("%s/%s.html", testServer.url, fixture)
	if _, err := page.Goto(url); err != nil {
		t.Fatalf("goto %s: %v", url, err)
	}

	// Wait for fixture to complete.
	logSel := page.Locator("#log")
	if err := logSel.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(30000),
	}); err != nil {
		t.Fatalf("wait for #log visible: %v", err)
	}

	// Wait for "DONE" text.
	if err := playwright.NewPlaywrightAssertions().Locator(logSel).ToContainText("DONE", playwright.LocatorAssertionsToContainTextOptions{
		Timeout: playwright.Float(30000),
	}); err != nil {
		text, _ := logSel.TextContent()
		t.Fatalf("fixture did not complete (text=%q): %v", text, err)
	}

	// Extract results.
	results, err := page.Evaluate("window.__results")
	if err != nil {
		t.Fatalf("evaluate window.__results: %v", err)
	}

	resultsMap, ok := results.(map[string]interface{})
	if !ok {
		t.Fatalf("window.__results is not an object: %T", results)
	}

	return resultsMap
}

// TestDetect verifies feature detection probes across all 3 browsers.
func TestDetect(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "detect")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("detection failed: %v", results["detail"])
			}

			caps, ok := results["caps"].(map[string]interface{})
			if !ok {
				t.Fatalf("caps not a map: %T", results["caps"])
			}

			config, _ := results["config"].(string)
			t.Logf("config=%s desc=%s", config, results["configDesc"])

			// All browsers should detect crossOriginIsolated (server sets COOP+COEP).
			assertBoolCap(t, caps, "crossOriginIsolated", true)

			// All browsers should have BroadcastChannel.
			assertBoolCap(t, caps, "broadcastChannelAvailable", true)

			// All browsers should have WebLocks.
			assertBoolCap(t, caps, "webLocksAvailable", true)

			// SAB should be available on all browsers when cross-origin isolated.
			assertBoolCap(t, caps, "sabAvailable", true)

			// OPFS available on Chromium and Firefox over http://127.0.0.1.
			// WebKit does not grant OPFS to http://127.0.0.1 (requires https
			// or localhost). This is a known WebKit limitation.
			if browser != "webkit" {
				assertBoolCap(t, caps, "opfsAvailable", true)
			} else {
				opfs, _ := caps["opfsAvailable"].(bool)
				t.Logf("webkit opfsAvailable=%v (may be false on http://127.0.0.1)", opfs)
			}

			// Config verification per browser.
			switch browser {
			case "chromium", "firefox":
				// Should get Config C (SAB + OPFS snapshot recovery).
				if config != "C" {
					t.Errorf("expected config C on %s, got %s", browser, config)
				}
			case "webkit":
				// WebKit: SAB available but no OPFS in headless mode.
				// Expects Config B (SAB without OPFS) or A (fallback).
				if config != "B" && config != "A" && config != "C" {
					t.Errorf("unexpected config on webkit: %s", config)
				}
			}
		})
	}
}

// TestSabRing verifies SabRingStream point-to-point communication.
// SAB requires cross-origin isolation, which is not viable on WebKit
// in this test environment, so we run on Chromium and Firefox only.
func TestSabRing(t *testing.T) {
	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "sab-ring")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("sab-ring failed: %v", results["detail"])
			}

			assertBoolResult(t, results, "sendRecv", true)
			assertBoolResult(t, results, "bidirectional", true)
			assertBoolResult(t, results, "close", true)

			t.Logf("detail: %s", results["detail"])
		})
	}
}

// TestSabBus verifies the SAB shared bus multi-endpoint communication.
// Tests unicast, relay, and broadcast message delivery.
func TestSabBus(t *testing.T) {
	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "sab-bus")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("sab-bus failed: %v", results["detail"])
			}

			assertBoolResult(t, results, "unicast", true)
			assertBoolResult(t, results, "relay", true)
			assertBoolResult(t, results, "broadcast", true)

			t.Logf("detail: %s", results["detail"])
		})
	}
}

// TestDedicatedWorker verifies DedicatedWorker hosting: plugin-host wrapper
// receives busSab + busPluginId, registers on bus, loads plugin script, and
// the plugin communicates over the bus.
func TestDedicatedWorker(t *testing.T) {
	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "dedicated")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("dedicated worker failed: %v", results["detail"])
			}

			assertBoolResult(t, results, "registered", true)
			assertBoolResult(t, results, "pluginStarted", true)
			assertBoolResult(t, results, "pluginReceived", true)
			assertBoolResult(t, results, "configReceived", true)

			t.Logf("detail: %s", results["detail"])
		})
	}
}

// TestTransportFactory verifies that the transport factory selects the correct
// transports based on detected worker comms config.
func TestTransportFactory(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "transport")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("transport factory failed: %v", results["detail"])
			}

			assertBoolResult(t, results, "factoryCreated", true)

			config, _ := results["config"].(string)
			hasBus, _ := results["hasBusStream"].(bool)

			// SAB bus is available on any config with SAB (B, C).
			// Config A/F have no SAB and no bus stream.
			switch config {
			case "B", "C":
				if !hasBus {
					t.Errorf("expected hasBusStream=true on %s (config=%s)", browser, config)
				}
			case "A", "F":
				if hasBus {
					t.Errorf("unexpected hasBusStream=true on %s (config=%s)", browser, config)
				}
			}

			t.Logf("config=%s hasBusStream=%v", config, hasBus)
		})
	}
}

// waitForDone waits for #log to show "DONE".
func waitForDone(t *testing.T, page playwright.Page, label string) {
	t.Helper()
	logSel := page.Locator("#log")
	if err := logSel.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(30000),
	}); err != nil {
		t.Fatalf("%s: wait for #log visible: %v", label, err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(logSel).ToContainText("DONE", playwright.LocatorAssertionsToContainTextOptions{
		Timeout: playwright.Float(30000),
	}); err != nil {
		text, _ := logSel.TextContent()
		t.Fatalf("%s: fixture did not complete (text=%q): %v", label, text, err)
	}
}

// toFloat64 converts a Playwright evaluate result to float64.
// Playwright-Go may return int, float64, or json.Number depending on the value.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// extractResults reads window.__results from a page.
func extractResults(t *testing.T, page playwright.Page) map[string]interface{} {
	t.Helper()
	results, err := page.Evaluate("window.__results")
	if err != nil {
		t.Fatalf("evaluate window.__results: %v", err)
	}
	resultsMap, ok := results.(map[string]interface{})
	if !ok {
		t.Fatalf("window.__results is not an object: %T", results)
	}
	return resultsMap
}

// assertBoolResult asserts a bool field in the results map.
func assertBoolResult(t *testing.T, results map[string]interface{}, key string, expected bool) {
	t.Helper()
	val, ok := results[key].(bool)
	if !ok {
		t.Errorf("result %s: not a bool (%T)", key, results[key])
		return
	}
	if val != expected {
		t.Errorf("result %s: got %v, want %v", key, val, expected)
	}
}

// assertBoolCap asserts a capability value in the caps map.
func assertBoolCap(t *testing.T, caps map[string]interface{}, key string, expected bool) {
	t.Helper()
	val, ok := caps[key].(bool)
	if !ok {
		t.Errorf("cap %s: not a bool (%T)", key, caps[key])
		return
	}
	if val != expected {
		t.Errorf("cap %s: got %v, want %v", key, val, expected)
	}
}

// TestCrossTab verifies cross-tab MessagePort brokering via ServiceWorker.
// Two pages in the same browser context each register with the SW and get
// a direct MessagePort channel to the other. Messages flow directly.
func TestCrossTab(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			bt := browserType(browser)
			bw, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer bw.Close()

			ctx, err := bw.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			// Open two pages (tabs) in the same context.
			pageA, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page A: %v", err)
			}
			pageA.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s A console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageA.On("pageerror", func(err error) {
				t.Logf("[%s A pageerror] %s", browser, err.Error())
			})

			pageB, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page B: %v", err)
			}
			pageB.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s B console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageB.On("pageerror", func(err error) {
				t.Logf("[%s B pageerror] %s", browser, err.Error())
			})

			url := fmt.Sprintf("%s/cross-tab.html", testServer.url)

			// Navigate page A first, wait for SW registration.
			if _, err := pageA.Goto(url); err != nil {
				t.Fatalf("goto A: %v", err)
			}
			waitForDone(t, pageA, "page A")

			resultsA := extractResults(t, pageA)
			assertBoolResult(t, resultsA, "swRegistered", true)

			// Navigate page B, wait for SW registration.
			if _, err := pageB.Goto(url); err != nil {
				t.Fatalf("goto B: %v", err)
			}
			waitForDone(t, pageB, "page B")

			resultsB := extractResults(t, pageB)
			assertBoolResult(t, resultsB, "swRegistered", true)

			// Wait for both pages to receive a peer channel.
			// Page B's "hello" triggers channel creation for both.
			err = playwright.NewPlaywrightAssertions().Page(pageA).ToHaveTitle("cross-tab", playwright.PageAssertionsToHaveTitleOptions{
				Timeout: playwright.Float(100),
			})
			_ = err // ignore title check, just need a small delay

			// Poll for peerCount on both pages.
			for i := 0; i < 50; i++ {
				rA := extractResults(t, pageA)
				rB := extractResults(t, pageB)
				pcA := toFloat64(rA["peerCount"])
				pcB := toFloat64(rB["peerCount"])
				if pcA >= 1 && pcB >= 1 {
					break
				}
				pageA.WaitForTimeout(100)
			}

			resultsA = extractResults(t, pageA)
			resultsB = extractResults(t, pageB)

			pcA := toFloat64(resultsA["peerCount"])
			pcB := toFloat64(resultsB["peerCount"])
			if pcA < 1 {
				t.Fatalf("page A: expected peerCount >= 1, got %v", resultsA["peerCount"])
			}
			if pcB < 1 {
				t.Fatalf("page B: expected peerCount >= 1, got %v", pcB)
			}

			// Send a message from A to B.
			if _, err := pageA.Evaluate("window.sendToPeers('hello from A')"); err != nil {
				t.Fatalf("sendToPeers A: %v", err)
			}

			// Send a message from B to A.
			if _, err := pageB.Evaluate("window.sendToPeers('hello from B')"); err != nil {
				t.Fatalf("sendToPeers B: %v", err)
			}

			// Wait for messages to arrive.
			pageA.WaitForTimeout(500)

			resultsA = extractResults(t, pageA)
			resultsB = extractResults(t, pageB)

			msgsA, _ := resultsA["messagesReceived"].([]interface{})
			msgsB, _ := resultsB["messagesReceived"].([]interface{})

			if len(msgsA) < 1 {
				t.Errorf("page A: expected >= 1 message, got %d", len(msgsA))
			} else {
				t.Logf("page A received: %v", msgsA)
			}

			if len(msgsB) < 1 {
				t.Errorf("page B: expected >= 1 message, got %d", len(msgsB))
			} else {
				t.Logf("page B received: %v", msgsB)
			}
		})
	}
}

// TestCrossTabCleanup verifies that clients.matchAll() excludes closed tabs.
// Opens 3 pages (all-to-all channels), closes one, opens a new page, and
// verifies the new page only gets channels to the 2 remaining pages.
func TestCrossTabCleanup(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			bt := browserType(browser)
			bw, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer bw.Close()

			ctx, err := bw.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			url := fmt.Sprintf("%s/cross-tab.html", testServer.url)

			// Helper to create a page with console logging.
			newPage := func(label string) playwright.Page {
				p, err := ctx.NewPage()
				if err != nil {
					t.Fatalf("new page %s: %v", label, err)
				}
				p.On("console", func(msg playwright.ConsoleMessage) {
					t.Logf("[%s %s console.%s] %s", browser, label, msg.Type(), msg.Text())
				})
				return p
			}

			// Open 3 pages sequentially.
			pageA := newPage("A")
			if _, err := pageA.Goto(url); err != nil {
				t.Fatalf("goto A: %v", err)
			}
			waitForDone(t, pageA, "page A")

			pageB := newPage("B")
			if _, err := pageB.Goto(url); err != nil {
				t.Fatalf("goto B: %v", err)
			}
			waitForDone(t, pageB, "page B")

			pageC := newPage("C")
			if _, err := pageC.Goto(url); err != nil {
				t.Fatalf("goto C: %v", err)
			}
			waitForDone(t, pageC, "page C")

			// Wait for all-to-all channels (each page should have 2 peers).
			pollPeerCount := func(pages []playwright.Page, labels []string, expected int) {
				for i := 0; i < 50; i++ {
					allReady := true
					for j, p := range pages {
						r := extractResults(t, p)
						pc := toFloat64(r["peerCount"])
						if pc < float64(expected) {
							allReady = false
							break
						}
						_ = labels[j]
					}
					if allReady {
						return
					}
					pages[0].WaitForTimeout(100)
				}
				for j, p := range pages {
					r := extractResults(t, p)
					t.Logf("page %s: peerCount=%v", labels[j], r["peerCount"])
				}
				t.Fatalf("not all pages reached peerCount >= %d", expected)
			}

			pollPeerCount(
				[]playwright.Page{pageA, pageB, pageC},
				[]string{"A", "B", "C"},
				2,
			)

			// Close page C.
			if err := pageC.Close(); err != nil {
				t.Fatalf("close C: %v", err)
			}

			// Open a new page D.
			pageD := newPage("D")
			if _, err := pageD.Goto(url); err != nil {
				t.Fatalf("goto D: %v", err)
			}
			waitForDone(t, pageD, "page D")

			// D should get channels to A and B only (not C which is closed).
			// clients.matchAll() returns only live clients.
			pollPeerCount(
				[]playwright.Page{pageD},
				[]string{"D"},
				2,
			)

			resultsD := extractResults(t, pageD)
			pcD := toFloat64(resultsD["peerCount"])
			if pcD != 2 {
				t.Errorf("page D: expected peerCount == 2, got %v", pcD)
			}

			// Verify D can exchange messages with the remaining pages.
			if _, err := pageD.Evaluate("window.sendToPeers('hello from D')"); err != nil {
				t.Fatalf("sendToPeers D: %v", err)
			}
			pageA.WaitForTimeout(300)

			// A and B should receive D's message.
			rA := extractResults(t, pageA)
			rB := extractResults(t, pageB)
			msgsA, _ := rA["messagesReceived"].([]interface{})
			msgsB, _ := rB["messagesReceived"].([]interface{})

			// A and B may have earlier messages from the all-to-all phase,
			// so just check the latest includes D's message.
			foundA := false
			for _, m := range msgsA {
				if s, ok := m.(string); ok && s == `{"text":"hello from D"}` {
					foundA = true
				}
			}
			foundB := false
			for _, m := range msgsB {
				if s, ok := m.(string); ok && s == `{"text":"hello from D"}` {
					foundB = true
				}
			}
			if !foundA {
				t.Errorf("page A did not receive message from D, msgs: %v", msgsA)
			}
			if !foundB {
				t.Errorf("page B did not receive message from D, msgs: %v", msgsB)
			}
		})
	}
}

// TestCrossTabSWRestart verifies that existing MessagePorts survive
// ServiceWorker unregister/re-register, and new tabs get fresh channels.
func TestCrossTabSWRestart(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			bt := browserType(browser)
			bw, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer bw.Close()

			ctx, err := bw.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			url := fmt.Sprintf("%s/cross-tab.html", testServer.url)

			newPage := func(label string) playwright.Page {
				p, err := ctx.NewPage()
				if err != nil {
					t.Fatalf("new page %s: %v", label, err)
				}
				p.On("console", func(msg playwright.ConsoleMessage) {
					t.Logf("[%s %s console.%s] %s", browser, label, msg.Type(), msg.Text())
				})
				return p
			}

			// Open two pages and establish channels.
			pageA := newPage("A")
			if _, err := pageA.Goto(url); err != nil {
				t.Fatalf("goto A: %v", err)
			}
			waitForDone(t, pageA, "page A")

			pageB := newPage("B")
			if _, err := pageB.Goto(url); err != nil {
				t.Fatalf("goto B: %v", err)
			}
			waitForDone(t, pageB, "page B")

			// Wait for channel establishment.
			for i := 0; i < 50; i++ {
				rA := extractResults(t, pageA)
				rB := extractResults(t, pageB)
				if toFloat64(rA["peerCount"]) >= 1 && toFloat64(rB["peerCount"]) >= 1 {
					break
				}
				pageA.WaitForTimeout(100)
			}

			// Verify messages flow before SW restart.
			if _, err := pageA.Evaluate("window.sendToPeers('pre-restart')"); err != nil {
				t.Fatalf("sendToPeers A: %v", err)
			}
			pageB.WaitForTimeout(300)

			rB := extractResults(t, pageB)
			msgsB, _ := rB["messagesReceived"].([]interface{})
			if len(msgsB) < 1 {
				t.Fatalf("page B: no messages before SW restart")
			}

			// Unregister and re-register the ServiceWorker.
			_, err = pageA.Evaluate(`(async () => {
				const regs = await navigator.serviceWorker.getRegistrations();
				for (const reg of regs) { await reg.unregister(); }
				const reg = await navigator.serviceWorker.register('/cross-tab-sw.js');
				const sw = reg.installing || reg.waiting || reg.active;
				if (!sw) throw new Error('no SW after re-register');
				await new Promise(resolve => {
					if (sw.state === 'activated') { resolve(); return; }
					sw.addEventListener('statechange', () => {
						if (sw.state === 'activated') resolve();
					});
				});
			})()`)
			if err != nil {
				t.Fatalf("SW restart: %v", err)
			}

			// Existing ports should still work (they are direct tab-to-tab).
			if _, err := pageA.Evaluate("window.sendToPeers('post-restart')"); err != nil {
				t.Fatalf("sendToPeers A post-restart: %v", err)
			}
			pageB.WaitForTimeout(300)

			rB = extractResults(t, pageB)
			msgsB, _ = rB["messagesReceived"].([]interface{})
			found := false
			for _, m := range msgsB {
				if s, ok := m.(string); ok && s == `{"text":"post-restart"}` {
					found = true
				}
			}
			if !found {
				t.Errorf("page B did not receive post-restart message, msgs: %v", msgsB)
			}

			// New tab connecting after restart should get fresh channels.
			pageC := newPage("C")
			if _, err := pageC.Goto(url); err != nil {
				t.Fatalf("goto C: %v", err)
			}
			waitForDone(t, pageC, "page C")

			// Wait for C to get peers.
			for i := 0; i < 50; i++ {
				rC := extractResults(t, pageC)
				if toFloat64(rC["peerCount"]) >= 1 {
					break
				}
				pageC.WaitForTimeout(100)
			}

			rC := extractResults(t, pageC)
			pcC := toFloat64(rC["peerCount"])
			if pcC < 1 {
				t.Errorf("page C: expected peerCount >= 1 after SW restart, got %v", pcC)
			}

			// Verify C can send messages.
			if _, err := pageC.Evaluate("window.sendToPeers('from C')"); err != nil {
				t.Fatalf("sendToPeers C: %v", err)
			}
			pageA.WaitForTimeout(300)

			rA := extractResults(t, pageA)
			msgsA, _ := rA["messagesReceived"].([]interface{})
			foundC := false
			for _, m := range msgsA {
				if s, ok := m.(string); ok && s == `{"text":"from C"}` {
					foundC = true
				}
			}
			if !foundC {
				t.Errorf("page A did not receive message from C after SW restart, msgs: %v", msgsA)
			}
		})
	}
}

// TestCrossTabRpc verifies StarPC echo RPC over brokered cross-tab
// MessagePort channels. Two pages each run a StarPC echo server on
// incoming relay connections. Page A opens a sub-channel to B and
// calls Echo. All 3 browsers.
func TestCrossTabRpc(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			bt := browserType(browser)
			bw, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer bw.Close()

			ctx, err := bw.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			url := fmt.Sprintf("%s/cross-tab-rpc.html", testServer.url)

			pageA, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page A: %v", err)
			}
			pageA.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s A console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageA.On("pageerror", func(err error) {
				t.Logf("[%s A pageerror] %s", browser, err.Error())
			})

			pageB, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page B: %v", err)
			}
			pageB.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s B console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageB.On("pageerror", func(err error) {
				t.Logf("[%s B pageerror] %s", browser, err.Error())
			})

			// Navigate both pages.
			if _, err := pageA.Goto(url); err != nil {
				t.Fatalf("goto A: %v", err)
			}
			waitForDone(t, pageA, "page A")

			if _, err := pageB.Goto(url); err != nil {
				t.Fatalf("goto B: %v", err)
			}
			waitForDone(t, pageB, "page B")

			// Wait for both pages to have peers.
			for i := 0; i < 50; i++ {
				rA := extractResults(t, pageA)
				rB := extractResults(t, pageB)
				if toFloat64(rA["peerCount"]) >= 1 && toFloat64(rB["peerCount"]) >= 1 {
					break
				}
				pageA.WaitForTimeout(100)
			}

			rA := extractResults(t, pageA)
			rB := extractResults(t, pageB)
			if toFloat64(rA["peerCount"]) < 1 {
				t.Fatalf("page A: no peers")
			}
			if toFloat64(rB["peerCount"]) < 1 {
				t.Fatalf("page B: no peers")
			}

			// Get B's peer ID as seen by A.
			peerIDs, err := pageA.Evaluate("Array.from(window.__peers?.keys?.() ?? [])")
			if err != nil {
				// Fall back: read the peer ID from the results on page B.
				t.Logf("could not read peers from A: %v", err)
			}
			peerIDList, _ := peerIDs.([]interface{})
			if len(peerIDList) == 0 {
				t.Fatalf("page A has no peer IDs")
			}
			targetPeerId, _ := peerIDList[0].(string)
			t.Logf("calling Echo from A to B (peer %s)", targetPeerId)

			// Call Echo from A to B.
			result, err := pageA.Evaluate(fmt.Sprintf(
				"window.callEcho(%q, 'hello cross-tab')",
				targetPeerId,
			))
			if err != nil {
				t.Fatalf("callEcho: %v", err)
			}

			body, _ := result.(string)
			if body != "hello cross-tab" {
				t.Errorf("unexpected echo body: %q", body)
			}
			t.Logf("echo response: %q", body)
		})
	}
}

// TestTransportStreams verifies the transport factory openBusStream().
// On Config B/C (Chromium, Firefox): creates a bus stream, sends data, verifies
// round-trip through the factory's returned PacketStream.
// On Config A/F (WebKit): verifies openBusStream is unavailable.
func TestTransportStreams(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "transport-streams")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("transport-streams failed: %v", results["detail"])
			}

			config, _ := results["config"].(string)
			t.Logf("config=%s", config)

			switch browser {
			case "chromium", "firefox":
				assertBoolResult(t, results, "hasBusStream", true)
				assertBoolResult(t, results, "busStreamRoundTrip", true)
			case "webkit":
				// WebKit gets Config B (SAB available) or A (fallback).
				hasBus, _ := results["hasBusStream"].(bool)
				if config == "A" || config == "F" {
					if hasBus {
						t.Errorf("expected no bus stream on config %s", config)
					}
				}
			}

			t.Logf("detail: %s", results["detail"])
		})
	}
}

// TestSabRpc verifies StarPC echo RPC over SabBusStream between two
// DedicatedWorkers. Server and client each register on a shared SAB bus,
// open SabBusStreams to each other, and run a full StarPC Echo round-trip.
// Chromium + Firefox only (SAB requires cross-origin isolation).
func TestSabRpc(t *testing.T) {
	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "sab-rpc")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("sab-rpc failed: %v", results["detail"])
			}

			echoBody, _ := results["echoBody"].(string)
			if echoBody != "hello via SAB bus" {
				t.Errorf("unexpected echo body: %q", echoBody)
			}

			t.Logf("detail: %s", results["detail"])
		})
	}
}

// TestConfigAFallback verifies that without Cross-Origin Isolation headers,
// detection returns Config A (no SAB, no SharedWorker hosting for plugins).
// Uses a separate test server without COOP/COEP headers.
func TestConfigAFallback(t *testing.T) {
	noCOIServer, err := newTestServerNoCOI(distDir)
	if err != nil {
		t.Fatalf("create no-COI server: %v", err)
	}
	defer noCOIServer.Close()
	noCOIURL := "http://" + noCOIServer.Listener.Addr().String()

	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			bt := browserType(browser)
			bw, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer bw.Close()

			ctx, err := bw.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			page, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page: %v", err)
			}
			page.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s console.%s] %s", browser, msg.Type(), msg.Text())
			})

			url := fmt.Sprintf("%s/detect.html", noCOIURL)
			if _, err := page.Goto(url); err != nil {
				t.Fatalf("goto: %v", err)
			}

			logSel := page.Locator("#log")
			if err := playwright.NewPlaywrightAssertions().Locator(logSel).ToContainText("DONE", playwright.LocatorAssertionsToContainTextOptions{
				Timeout: playwright.Float(30000),
			}); err != nil {
				t.Fatalf("fixture did not complete: %v", err)
			}

			results := extractResults(t, page)

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("detection failed: %v", results["detail"])
			}

			config, _ := results["config"].(string)
			caps, _ := results["caps"].(map[string]interface{})

			// Without COI headers, crossOriginIsolated should be false.
			coi, _ := caps["crossOriginIsolated"].(bool)
			if coi {
				t.Errorf("expected crossOriginIsolated=false without COI headers")
			}

			// SAB should be unavailable without cross-origin isolation.
			sab, _ := caps["sabAvailable"].(bool)
			if sab {
				t.Errorf("expected sabAvailable=false without COI headers")
			}

			// Config should be A or F (fallback).
			if config != "A" && config != "F" {
				t.Errorf("expected config A or F, got %s", config)
			}

			t.Logf("config=%s (without COI headers)", config)
		})
	}
}
