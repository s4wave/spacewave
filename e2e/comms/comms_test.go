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
	srv := newTestServer(distDir)
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

// TestSqliteComms verifies sqlite cross-tab communication.
// Tests single-page round-trip and cross-tab write/read via OPFS.
func TestSqliteComms(t *testing.T) {
	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser+"/single", func(t *testing.T) {
			results := runFixture(t, browser, "sqlite-comms")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("sqlite-comms single failed: %v", results["detail"])
			}

			assertBoolResult(t, results, "roundTrip", true)
			assertBoolResult(t, results, "bcNotification", true)

			t.Logf("detail: %s", results["detail"])
		})

		t.Run(browser+"/cross-tab", func(t *testing.T) {
			bt := browserType(browser)
			b, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer b.Close()

			ctx, err := b.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			// Page A: writer - writes message to OPFS.
			pageA, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page A: %v", err)
			}
			pageA.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s writer console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageA.On("pageerror", func(err error) {
				t.Logf("[%s writer pageerror] %s", browser, err.Error())
			})

			writerURL := fmt.Sprintf("%s/sqlite-comms.html?mode=writer", testServer.url)
			if _, err := pageA.Goto(writerURL); err != nil {
				t.Fatalf("goto writer: %v", err)
			}

			// Wait for writer to complete.
			waitForDone(t, pageA, browser+" writer")

			writerResults := extractResults(t, pageA)
			if pass, ok := writerResults["pass"].(bool); !ok || !pass {
				t.Fatalf("writer failed: %v", writerResults["detail"])
			}

			// Page B: reader - reads from OPFS via deserialization.
			pageB, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page B: %v", err)
			}
			pageB.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s reader console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageB.On("pageerror", func(err error) {
				t.Logf("[%s reader pageerror] %s", browser, err.Error())
			})

			readerURL := fmt.Sprintf("%s/sqlite-comms.html?mode=reader", testServer.url)
			if _, err := pageB.Goto(readerURL); err != nil {
				t.Fatalf("goto reader: %v", err)
			}

			waitForDone(t, pageB, browser+" reader")

			readerResults := extractResults(t, pageB)
			if pass, ok := readerResults["pass"].(bool); !ok || !pass {
				t.Fatalf("reader failed: %v", readerResults["detail"])
			}

			assertBoolResult(t, readerResults, "crossTabRead", true)
			t.Logf("detail: %s", readerResults["detail"])
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

// TestRecovery verifies orphan detection and snapshot recovery across page close.
// Page A acquires lock + writes snapshot, then Go closes it. Page B detects
// the orphan and recovers the snapshot.
func TestRecovery(t *testing.T) {
	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			bt := browserType(browser)
			b, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
				Headless: playwright.Bool(true),
			})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			defer b.Close()

			ctx, err := b.NewContext()
			if err != nil {
				t.Fatalf("new context: %v", err)
			}
			defer ctx.Close()

			// Page A: setup - acquire lock, write snapshot.
			pageA, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page A: %v", err)
			}
			pageA.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s setup console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageA.On("pageerror", func(err error) {
				t.Logf("[%s setup pageerror] %s", browser, err.Error())
			})

			setupURL := fmt.Sprintf("%s/recovery.html?mode=setup", testServer.url)
			if _, err := pageA.Goto(setupURL); err != nil {
				t.Fatalf("goto setup: %v", err)
			}
			waitForDone(t, pageA, browser+" setup")

			setupResults := extractResults(t, pageA)
			if pass, ok := setupResults["pass"].(bool); !ok || !pass {
				t.Fatalf("setup failed: %v", setupResults["detail"])
			}

			// Close page A to release the WebLock.
			if err := pageA.Close(); err != nil {
				t.Fatalf("close page A: %v", err)
			}

			// Page B: recover - find orphans, restore snapshot.
			pageB, err := ctx.NewPage()
			if err != nil {
				t.Fatalf("new page B: %v", err)
			}
			pageB.On("console", func(msg playwright.ConsoleMessage) {
				t.Logf("[%s recover console.%s] %s", browser, msg.Type(), msg.Text())
			})
			pageB.On("pageerror", func(err error) {
				t.Logf("[%s recover pageerror] %s", browser, err.Error())
			})

			recoverURL := fmt.Sprintf("%s/recovery.html?mode=recover", testServer.url)
			if _, err := pageB.Goto(recoverURL); err != nil {
				t.Fatalf("goto recover: %v", err)
			}
			waitForDone(t, pageB, browser+" recover")

			recoverResults := extractResults(t, pageB)
			if pass, ok := recoverResults["pass"].(bool); !ok || !pass {
				t.Fatalf("recover failed: %v", recoverResults["detail"])
			}

			assertBoolResult(t, recoverResults, "orphanDetected", true)
			assertBoolResult(t, recoverResults, "recovered", true)

			t.Logf("detail: %s", recoverResults["detail"])
		})
	}
}

// TestSnapshot verifies SnapshotManager snapshot/restore, dirty tracking, and
// listSnapshots. OPFS path on Chromium/Firefox, IDB fallback on WebKit.
func TestSnapshot(t *testing.T) {
	browsers := []string{"chromium", "firefox", "webkit"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			results := runFixture(t, browser, "snapshot")

			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("snapshot failed: %v", results["detail"])
			}

			assertBoolResult(t, results, "snapshotRestore", true)
			assertBoolResult(t, results, "dirtyTracking", true)
			assertBoolResult(t, results, "listSnapshots", true)

			t.Logf("detail: %s", results["detail"])
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
