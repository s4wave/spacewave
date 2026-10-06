//go:build !skip_e2e && !js

package comms

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// reliabilityFixture is the page that runs the reliability modes.
const reliabilityFixture = "goscript-volume-replay"

// markerTimeout bounds each wait for a worker marker.
const markerTimeout = 90 * time.Second

// reliabilityEngines are the volume engines the reliability scenarios run
// on, each on OPFS.
var reliabilityEngines = []string{"e1", "s4db"}

// TestGoScriptVolumeReliability runs each OPFS volume engine through the
// browser failures it must survive: a killed browser, a closed page, a full
// quota, eviction at rest and while open, and a second opener.
func TestGoScriptVolumeReliability(t *testing.T) {
	ensureGoScriptFixtureWorker(t, &volumeReplayGoScriptFixtureWorker)
	for _, browser := range volumeBrowsers(t) {
		t.Run(browser, func(t *testing.T) {
			// Skip Safari, which cannot kill, observe, or coordinate a page.
			if browser == "safari" {
				t.Skip("Safari's WebDriver cannot kill, observe, or coordinate a page")
			}
			for _, engine := range reliabilityEngines {
				t.Run(engine, func(t *testing.T) { testReliabilityEngine(t, rel{browser: browser, engine: engine}) })
			}
		})
	}
}

// rel is the browser and volume engine a reliability scenario runs on.
type rel struct {
	// browser is the browser running the page.
	browser string
	// engine is the volume engine the page opens.
	engine string
}

// testReliabilityEngine runs every reliability scenario r's browser supports.
func testReliabilityEngine(t *testing.T, r rel) {
	// Run the kill and page-close scenarios at each sync step.
	for _, at := range []int{40, 400} {
		// Killing the device browser would take down the operator's Chrome.
		if r.browser != "android" {
			t.Run("kill-"+strconv.Itoa(at), func(t *testing.T) { testReliabilityKill(t, r, at) })
		}
		t.Run("page-close-"+strconv.Itoa(at), func(t *testing.T) { testReliabilityPageClose(t, r, at) })
	}
	t.Run("evict-at-rest", func(t *testing.T) { testReliabilityEvictAtRest(t, r) })
	t.Run("contend", func(t *testing.T) { testReliabilityContend(t, r) })

	// Run the Chromium-only quota and open-volume eviction scenarios.
	if !chromiumFamily(r.browser) {
		return
	}
	t.Run("quota", func(t *testing.T) { testReliabilityQuota(t, r) })
	t.Run("evict-open", func(t *testing.T) { testReliabilityEvictOpen(t, r) })
}

// testReliabilityKill kills the browser once a burst syncs step at, then
// checks that a relaunch on the same profile recovers every synced step.
func testReliabilityKill(t *testing.T, r rel, at int) {
	// Launch a persistent browser context that reports its close.
	dir := t.TempDir()
	ctx := launchContext(t, r.browser, dir)
	closed := make(chan struct{})
	ctx.OnClose(func(playwright.BrowserContext) { close(closed) })

	// Sync a burst of steps, kill the browser, and wait for its exit.
	lines := newMarkers(at)
	startPage(t, ctx, r.browser, reliabilityFixture, r.run("burst:kill", lines))
	awaitClosed(t, "synced", lines.reached)
	if out, err := exec.Command("pkill", "-9", "-f", dir).CombinedOutput(); err != nil {
		t.Fatalf("kill %s: %v: %s", r.browser, err, out)
	}
	awaitClosed(t, "browser exit", closed)

	// Relaunch on the same profile and verify recovery from the synced step.
	synced := lines.lastSynced()
	ctx = launchContext(t, r.browser, dir)
	rep := r.runReliability(t, ctx, "verify:kill:"+strconv.Itoa(synced), nil)
	t.Logf("killed after synced %d, recovered head %d", synced, rep.head)
}

// testReliabilityPageClose closes the page once a burst syncs step at, then
// checks that a new page in the same browser recovers every synced step.
func testReliabilityPageClose(t *testing.T, r rel, at int) {
	// Sync a burst of steps in a new page and close that page.
	ctx := launchContext(t, r.browser, t.TempDir())
	lines := newMarkers(at)
	page, _ := startPage(t, ctx, r.browser, reliabilityFixture, r.run("burst:close", lines))
	awaitClosed(t, "synced", lines.reached)
	if err := page.Close(); err != nil {
		t.Fatalf("close page: %v", err)
	}

	// Open a new page in the same browser and verify recovery.
	synced := lines.lastSynced()
	rep := r.runReliability(t, ctx, "verify:close:"+strconv.Itoa(synced), nil)
	t.Logf("closed after synced %d, recovered head %d", synced, rep.head)
}

// testReliabilityEvictAtRest clears the origin's storage between sessions and
// checks that the volume reopens empty.
func testReliabilityEvictAtRest(t *testing.T, r rel) {
	// Write steps and then clear the origin's storage between sessions.
	ctx := launchContext(t, r.browser, t.TempDir())
	r.runReliability(t, ctx, "write:rest:40", nil)
	if chromiumFamily(r.browser) {
		page, cdp := cdpSession(t, ctx)
		cdpSend(t, cdp, "Storage.clearDataForOrigin", map[string]any{
			"origin":       testServer.url,
			"storageTypes": "all",
		})
		_ = page.Close()
	} else {
		r.runReliability(t, ctx, "wipe:rest", nil)
	}

	// Reopen the volume and expect it to start empty.
	r.runReliability(t, ctx, "reopen:rest:-1", nil)
}

// testReliabilityContend holds the volume on one page while a second page
// tries to open it, which must fail cleanly and leave the holder working.
func testReliabilityContend(t *testing.T, r rel) {
	// Hold the volume open on one page.
	ctx := launchContext(t, r.browser, t.TempDir())
	lines := newMarkers(-1)
	_, waitHolder := startPage(t, ctx, r.browser, reliabilityFixture, r.run("hold:contend", lines))
	awaitClosed(t, "held", lines.marker("held"))
	contender := r.runReliability(t, ctx, "contend:contend&instance=contender", nil)
	t.Logf("contender errors: %v", contender.errors)

	// Expect the holder page to keep working after the failed open.
	checkReliability(t, waitHolder())
}

// testReliabilityQuota fills the volume against a 4 MiB origin quota, then
// lifts the quota and checks that the volume keeps working.
func testReliabilityQuota(t *testing.T, r rel) {
	// Impose a small origin quota through the DevTools protocol.
	ctx := launchContext(t, r.browser, t.TempDir())
	_, cdp := cdpSession(t, ctx)
	cdpSend(t, cdp, "Storage.overrideQuotaForOrigin", map[string]any{
		"origin":    testServer.url,
		"quotaSize": 4 << 20,
	})

	// Fill the volume until the page hits the quota.
	lines := newMarkers(-1)
	page, wait := startPage(t, ctx, r.browser, reliabilityFixture, r.run("quota:quota", lines))
	awaitClosed(t, "quota-hit", lines.marker("quota-hit"))

	// Lift the quota and expect the volume to keep working.
	cdpSend(t, cdp, "Storage.overrideQuotaForOrigin", map[string]any{"origin": testServer.url})
	postReliability(t, page, "space-freed")

	// Expect the quota run to have reported write errors.
	rep := checkReliability(t, wait())
	if len(rep.errors) == 0 {
		t.Fatal("quota run reported no write error")
	}
	t.Logf("quota errors: %v", rep.errors)
}

// testReliabilityEvictOpen clears the origin's storage while the volume is
// open and checks that the open volume answers without crashing, then that a
// new page reopens it and takes writes. The evicted page is closed rather
// than closing the volume: Chromium's FileSystemSyncAccessHandle.close never
// returns after the origin is cleared under an open handle.
func testReliabilityEvictOpen(t *testing.T, r rel) {
	// Open the volume and clear the origin's storage under it.
	ctx := launchContext(t, r.browser, t.TempDir())
	_, cdp := cdpSession(t, ctx)
	lines := newMarkers(-1)
	page, wait := startPage(t, ctx, r.browser, reliabilityFixture, r.run("evict:open", lines))
	awaitClosed(t, "evict-ready", lines.marker("evict-ready"))
	cdpSend(t, cdp, "Storage.clearDataForOrigin", map[string]any{
		"origin":       testServer.url,
		"storageTypes": "all",
	})

	// Expect the open volume to answer after the eviction.
	postReliability(t, page, "evicted")
	rep := checkReliability(t, wait())
	t.Logf("evicted while open: head %d, errors %v", rep.head, rep.errors)

	// Close the evicted page.
	if err := page.Close(); err != nil {
		t.Fatalf("close page: %v", err)
	}

	// Reopen the volume in a new page and expect writes to land.
	rep = r.runReliability(t, ctx, "reopen:open:-2", nil)
	t.Logf("reopened after eviction: head %d", rep.head)
}

// chromiumFamily reports whether browser speaks the Chrome DevTools Protocol.
func chromiumFamily(browser string) bool {
	// Match the browsers speaking the Chrome DevTools Protocol.
	return browser == "chromium" || browser == "chrome" || browser == "android"
}

// run is the page run of one reliability mode on r's engine, feeding its
// console lines to lines when set.
func (r rel) run(mode string, lines *markers) fixtureRun {
	// Configure a persistent page run with the reliability mode and timeout.
	run := fixtureRun{
		persistent: true,
		query:      "mode=reliability:" + r.engine + ":" + mode,
		timeout:    110 * time.Second,
	}
	if lines != nil {
		run.onLine = lines.observe
	}
	return run
}

// runReliability runs one reliability mode to completion in a new page of
// ctx and returns its checked report.
func (r rel) runReliability(t *testing.T, ctx playwright.BrowserContext, mode string, lines *markers) relReport {
	// Run the mode in a new page and check its report.
	t.Helper()
	return checkReliability(t, runPage(t, ctx, r.browser, reliabilityFixture, r.run(mode, lines)))
}

// relReport is the part of a reliability report the checks read.
type relReport struct {
	head   int
	errors []string
}

// checkReliability fails t unless the page passed and returns its report.
func checkReliability(t *testing.T, results map[string]any) relReport {
	// Fail unless the page reported a pass.
	t.Helper()
	if pass, ok := results["pass"].(bool); !ok || !pass {
		t.Fatalf("reliability fixture failed: %v", results["detail"])
	}

	// Extract the head and error list from the report.
	report := parseReport(t, results)
	rep := relReport{head: report.GetInt("head")}
	for _, msg := range report.GetArray("errors") {
		rep.errors = append(rep.errors, string(msg.GetStringBytes()))
	}
	return rep
}

// cdpSession opens a blank page in ctx and a DevTools session on it.
func cdpSession(t *testing.T, ctx playwright.BrowserContext) (playwright.Page, playwright.CDPSession) {
	// Open a blank page and attach a DevTools session to it.
	t.Helper()
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	cdp, err := ctx.NewCDPSession(page)
	if err != nil {
		t.Fatalf("cdp session: %v", err)
	}
	return page, cdp
}

// cdpSend sends one DevTools command.
func cdpSend(t *testing.T, cdp playwright.CDPSession, method string, params map[string]any) {
	// Send one DevTools command and fail the test on error.
	t.Helper()
	if _, err := cdp.Send(method, params); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

// postReliability posts msg on the channel the reliability worker waits on.
func postReliability(t *testing.T, page playwright.Page, msg string) {
	// Post the message on the reliability broadcast channel.
	t.Helper()
	if _, err := page.Evaluate("msg => new BroadcastChannel('volume-reliability').postMessage(msg)", msg); err != nil {
		t.Fatalf("post %s: %v", msg, err)
	}
}

// awaitClosed waits for done, failing t after markerTimeout.
func awaitClosed(t *testing.T, what string, done <-chan struct{}) {
	// Wait for the channel or fail after the marker timeout.
	t.Helper()
	select {
	case <-done:
	case <-time.After(markerTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// markers watches a reliability page's console for the worker's markers:
// "synced <step>" after each Sync, and the named coordination points.
type markers struct {
	// at is the synced step that closes reached, -1 for none.
	at      int
	reached chan struct{}
	synced  atomic.Int64

	mu    sync.Mutex
	named map[string]chan struct{}
}

// newMarkers returns markers that close reached once a Sync covers at.
func newMarkers(at int) *markers {
	// Start the synced counter below every step.
	m := &markers{at: at, reached: make(chan struct{}), named: make(map[string]chan struct{})}
	m.synced.Store(-1)
	return m
}

// marker returns the channel closed when the worker logs name.
func (m *markers) marker(name string) chan struct{} {
	// Return the named channel, creating it on first use.
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.named[name]
	if !ok {
		ch = make(chan struct{})
		m.named[name] = ch
	}
	return ch
}

// observe reads one console line.
func (m *markers) observe(line string) {
	// Ignore lines from other sources.
	_, text, ok := strings.Cut(line, "goscript-volume-replay: ")
	if !ok {
		return
	}

	// Route synced markers to the counter and close the named channel.
	word, arg, _ := strings.Cut(text, " ")
	if word == "synced" {
		m.observeSynced(arg)
		return
	}
	ch := m.marker(word)
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// observeSynced records a "synced <step>" line.
func (m *markers) observeSynced(arg string) {
	// Record the synced step and close reached at the threshold.
	step, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	m.synced.Store(int64(step))
	if m.at >= 0 && step >= m.at {
		m.mu.Lock()
		defer m.mu.Unlock()
		select {
		case <-m.reached:
		default:
			close(m.reached)
		}
	}
}

// lastSynced returns the last step a logged Sync covered.
func (m *markers) lastSynced() int {
	return int(m.synced.Load())
}
