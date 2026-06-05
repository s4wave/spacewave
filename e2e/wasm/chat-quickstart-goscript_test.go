//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const chatQuickstartWaitMS = 240000

func TestGoScriptChatQuickstartMessagingParity(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := testHarness.NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during Chat quickstart gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during Chat quickstart gate: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/chat")

	waitForChatRoute(t, page)
	route := page.URL()
	if !strings.Contains(route, "/-/chat/channel/general") {
		t.Fatalf("Chat quickstart route = %q, want /-/chat/channel/general; debug: %v", route, collectChatQuickstartDebug(page))
	}

	message := "GoScript Chat Proof"
	sendChatMessage(t, page, message)
	waitForChatMessage(t, page, message)

	if _, err := page.Reload(playwright.PageReloadOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(chatQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("reload Chat route: %v", err)
	}
	waitForChatMessage(t, page, message)

	NavigateHash(t, testHarness, page, "#/")
	hashIdx := strings.Index(route, "#")
	if hashIdx < 0 {
		t.Fatalf("Chat quickstart route has no hash: %q", route)
	}
	NavigateHash(t, testHarness, page, route[hashIdx:])
	waitForChatMessage(t, page, message)
}

func waitForChatRoute(t testing.TB, page playwright.Page) {
	t.Helper()

	locator := page.Locator("textarea[placeholder=\"Type a message...\"]")
	if err := locator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(chatQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("wait for Chat message input: %v\ndebug: %v", err, collectChatQuickstartDebug(page))
	}
	if err := page.Locator("text=Chat").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(chatQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("wait for Chat viewer header: %v\ndebug: %v", err, collectChatQuickstartDebug(page))
	}
}

func sendChatMessage(t testing.TB, page playwright.Page, message string) {
	t.Helper()

	input := page.Locator("textarea[placeholder=\"Type a message...\"]")
	if err := input.Fill(message, playwright.LocatorFillOptions{
		Timeout: playwright.Float(chatQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("fill Chat message: %v\ndebug: %v", err, collectChatQuickstartDebug(page))
	}
	if err := input.Press("Enter", playwright.LocatorPressOptions{
		Timeout: playwright.Float(chatQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("send Chat message: %v\ndebug: %v", err, collectChatQuickstartDebug(page))
	}
}

func waitForChatMessage(t testing.TB, page playwright.Page, message string) {
	t.Helper()

	if err := page.Locator("text=" + message).First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(chatQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("wait for Chat message %q: %v\ndebug: %v", message, err, collectChatQuickstartDebug(page))
	}
}

func collectChatQuickstartDebug(page playwright.Page) any {
	raw, err := page.Evaluate(`() => ({
		hash: window.location.hash,
		timing: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
		text: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 2200) ?? '',
		hasInput: !!document.querySelector('textarea[placeholder="Type a message..."]'),
	})`)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return raw
}
