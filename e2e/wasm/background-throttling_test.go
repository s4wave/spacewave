//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

var backgroundThrottleForbiddenConsole = []string{
	"timeout waiting for runtime connected ack",
	"timed out waiting for ack from WebDocument",
	"timed out waiting for ack",
	"stream-open-timeout",
}

// TestBackgroundThrottledLifecycle proves a browser tab can remain backgrounded
// beyond common timer-throttling windows and resume without timer-owned runtime
// lifecycle failures.
func TestBackgroundThrottledLifecycle(t *testing.T) {
	sess := testHarness.NewCleanPageSession(t)
	page := sess.Page()
	WaitForApp(t, page)
	AssertBrowserStartupDone(t, testHarness, page)

	console, stopConsole := sess.WatchConsole()
	consoleCollector := startBackgroundThrottleConsoleCollector(console)
	defer stopConsole()

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

	waitBackgroundThrottleWindow(t, 35*time.Second)

	if err := page.BringToFront(); err != nil {
		t.Fatalf("bring app page forward: %v", err)
	}
	WaitForApp(t, page)
	AssertBrowserStartupDone(t, testHarness, page)

	NavigateHash(t, testHarness, page, "#/quickstart/drive")
	WaitForDriveReady(t, testHarness, page)
	AssertBrowserStartupDone(t, testHarness, page)

	stopConsole()
	consoleCollector.Wait()
	assertNoBackgroundThrottleConsoleFailures(t, consoleCollector.Messages())
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
