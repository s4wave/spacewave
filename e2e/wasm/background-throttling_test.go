//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

var backgroundThrottleForbiddenConsole = []string{
	"timeout waiting for runtime connected ack",
	"timed out waiting for ack from WebDocument",
	"timed out waiting for ack",
	"stream-open-timeout",
}

const backgroundThrottleResumeFolder = "background-throttle-resume-proof"

// TestBackgroundThrottledLifecycle proves two app tabs can remain backgrounded
// beyond common timer-throttling windows and resume without document deletion,
// runtime reconnection churn, or stream loss.
func TestBackgroundThrottledLifecycle(t *testing.T) {
	h := harness(t)
	sess := h.NewCleanSession(t)
	scenario := CreateDriveScenario(t, h, sess)
	leftPage := scenario.GetSession().Page()
	WaitForDriveReady(t, h, leftPage)
	AssertBrowserStartupDone(t, h, leftPage)
	targetHash, err := currentHash(leftPage.URL())
	if err != nil {
		t.Fatalf("current drive hash: %v", err)
	}

	rightPage, err := h.newBrowserPage(sess)
	if err != nil {
		t.Fatalf("open second app document: %v", err)
	}
	h.registerPageSession(rightPage, sess)
	defer func() {
		h.unregisterPageSession(rightPage)
		if err := rightPage.Close(); err != nil {
			t.Errorf("close second app document: %v", err)
		}
	}()
	loadPageURL(t, rightPage, h.BaseURL()+"/"+targetHash)
	WaitForApp(t, rightPage)
	WaitForDriveReady(t, h, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)

	leftRuntimeConnected := startupMarkCount(t, leftPage, "runtime.connected")
	rightRuntimeConnected := startupMarkCount(t, rightPage, "runtime.connected")
	if leftRuntimeConnected == 0 || rightRuntimeConnected == 0 {
		t.Fatalf(
			"expected runtime connection marks before backgrounding; left=%d right=%d",
			leftRuntimeConnected,
			rightRuntimeConnected,
		)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	consoleCollector := startBackgroundThrottleConsoleCollector(console)

	foregroundPage, err := sess.BrowserContext().NewPage()
	if err != nil {
		t.Fatalf("open foreground page: %v", err)
	}
	defer foregroundPage.Close()
	if _, err := foregroundPage.Goto("about:blank"); err != nil {
		t.Fatalf("load foreground page: %v", err)
	}
	if err := foregroundPage.BringToFront(); err != nil {
		t.Fatalf("bring foreground page forward: %v", err)
	}

	waitBackgroundThrottleWindow(t, 60*time.Second)

	if err := leftPage.BringToFront(); err != nil {
		t.Fatalf("bring left app page forward: %v", err)
	}
	WaitForApp(t, leftPage)
	WaitForDriveReady(t, h, leftPage)
	AssertBrowserStartupDone(t, h, leftPage)
	assertStartupMarkCount(t, leftPage, "runtime.connected", leftRuntimeConnected)

	if err := rightPage.BringToFront(); err != nil {
		t.Fatalf("bring right app page forward: %v", err)
	}
	WaitForApp(t, rightPage)
	WaitForDriveReady(t, h, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)
	assertStartupMarkCount(t, rightPage, "runtime.connected", rightRuntimeConnected)

	createDriveFolder(t, leftPage, backgroundThrottleResumeFolder)
	waitForDriveEntry(t, rightPage, backgroundThrottleResumeFolder)

	stopConsole()
	consoleCollector.Wait()
	messages := consoleCollector.Messages()
	assertNoBackgroundThrottleConsoleFailures(t, messages)
	assertNoRemoteDocumentDeletedLog(t, messages)
}

// TestNeverFocusedBackgroundTabLoads proves a tab loaded behind a foreground
// page can open runtime streams before the test ever brings it to the front.
func TestNeverFocusedBackgroundTabLoads(t *testing.T) {
	h := harness(t)
	sess := h.NewCleanBlankSession(t)
	page := sess.Page()
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	consoleCollector := startBackgroundThrottleConsoleCollector(console)

	foregroundPage, err := sess.BrowserContext().NewPage()
	if err != nil {
		t.Fatalf("open foreground page: %v", err)
	}
	defer foregroundPage.Close()
	if _, err := foregroundPage.Goto("about:blank"); err != nil {
		t.Fatalf("load foreground page: %v", err)
	}
	if err := foregroundPage.BringToFront(); err != nil {
		t.Fatalf("bring foreground page forward: %v", err)
	}

	loadPageURL(t, page, h.BaseURL()+"/#/quickstart/drive")
	WaitForApp(t, page)
	WaitForDriveReady(t, h, page)
	AssertBrowserStartupDone(t, h, page)
	if startupMarkCount(t, page, "runtime.connected") == 0 {
		t.Fatal("background page opened the Drive route without a runtime connection mark")
	}

	stopConsole()
	consoleCollector.Wait()
	messages := consoleCollector.Messages()
	assertNoBackgroundThrottleConsoleFailures(t, messages)
	assertNoRemoteDocumentDeletedLog(t, messages)
}

func startupMarkCount(t testing.TB, page playwright.Page, label string) int {
	t.Helper()

	raw, err := page.Evaluate(`(arg) => {
		const label = Array.isArray(arg) ? arg[0] : arg
		return (globalThis.__swStartupMarks ?? [])
			.filter((mark) => mark.label === label)
			.length
	}`, []any{label})
	if err != nil {
		t.Fatalf("count startup marks %q: %v", label, err)
	}
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		t.Fatalf("unexpected startup mark count %T: %#v", raw, raw)
		return 0
	}
}

func assertStartupMarkCount(
	t testing.TB,
	page playwright.Page,
	label string,
	expected int,
) {
	t.Helper()

	if got := startupMarkCount(t, page, label); got != expected {
		t.Fatalf("startup mark count %q changed from %d to %d", label, expected, got)
	}
}

func waitBackgroundThrottleWindow(t testing.TB, d time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), d+5*time.Second)
	defer cancel()
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatalf("wait for background throttle window: %v", ctx.Err())
	}
}

type backgroundThrottleConsoleCollector struct {
	mu       sync.Mutex
	messages []string
	done     chan struct{}
}

func startBackgroundThrottleConsoleCollector(console <-chan string) *backgroundThrottleConsoleCollector {
	c := &backgroundThrottleConsoleCollector{done: make(chan struct{})}
	go func() {
		defer close(c.done)
		for msg := range console {
			c.mu.Lock()
			c.messages = append(c.messages, msg)
			c.mu.Unlock()
		}
	}()
	return c
}

func (c *backgroundThrottleConsoleCollector) Wait() {
	<-c.done
}

func (c *backgroundThrottleConsoleCollector) Messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.messages...)
}

func assertNoBackgroundThrottleConsoleFailures(t testing.TB, messages []string) {
	t.Helper()

	for _, msg := range messages {
		for _, forbidden := range backgroundThrottleForbiddenConsole {
			if strings.Contains(msg, forbidden) {
				t.Fatalf("background throttle console failure %q in %q", forbidden, msg)
			}
		}
	}
}
