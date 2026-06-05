//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const gitQuickstartWaitMS = 240000

func TestGoScriptGitQuickstartLocalCreateReloadParity(t *testing.T) {
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
			t.Errorf("unexpected browser/WASM crash report during Git quickstart gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during Git quickstart gate: %+v", report)
		}
	}()

	page := sess.Page()
	scenario := createGitQuickstartScenario(t, testHarness, page)
	repoHash := scenario.objectHash(scenario.repoObjectKey)
	worktreeObjectKey := scenario.repoObjectKey + "/worktree"
	worktreeHash := scenario.objectHash(worktreeObjectKey)

	waitForGitRepoViewerReady(t, page, scenario.repoObjectKey)
	assertGitRouteReloadAndReopen(t, testHarness, page, repoHash, func() {
		waitForGitRepoViewerReady(t, page, scenario.repoObjectKey)
	})

	NavigateHash(t, testHarness, page, worktreeHash)
	waitForGitWorktreeViewerReady(t, page, worktreeObjectKey)
	assertGitEmptyWorktreeHidesLogTab(t, page, worktreeObjectKey)
	assertGitRouteReloadAndReopen(t, testHarness, page, worktreeHash, func() {
		waitForGitWorktreeViewerReady(t, page, worktreeObjectKey)
		assertGitEmptyWorktreeHidesLogTab(t, page, worktreeObjectKey)
	})
}

type gitQuickstartScenario struct {
	sessionIndex  uint32
	spaceID       string
	repoObjectKey string
}

func createGitQuickstartScenario(
	t testing.TB,
	h *Harness,
	page playwright.Page,
) *gitQuickstartScenario {
	t.Helper()

	const repoName = "GoScript Git Row 8"
	const repoObjectKey = "goscript-git-row-8-1"

	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/git")
	waitForGitWizardReady(t, page)
	clickGitWizardText(t, page, "New empty repository")
	clickGitWizardText(t, page, "Next")
	repoNameInput := page.Locator("input[placeholder='Enter repository name...']").First()
	if err := repoNameInput.Fill(repoName); err != nil {
		t.Fatalf("fill Git repo name: %v\ndebug: %v", err, collectGitQuickstartDebug(page))
	}
	clickGitWizardText(t, page, "Create")
	waitForGitRepoViewerReady(t, page, repoObjectKey)

	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse Git quickstart route: %v", err)
	}
	return &gitQuickstartScenario{
		sessionIndex:  sessionIndex,
		spaceID:       spaceID,
		repoObjectKey: repoObjectKey,
	}
}

func (s *gitQuickstartScenario) objectHash(objectKey string) string {
	return fmt.Sprintf("#/u/%d/so/%s/-/%s", s.sessionIndex, s.spaceID, objectKey)
}

func waitForGitWizardReady(t testing.TB, page playwright.Page) {
	t.Helper()

	for _, text := range []string{"New Git Repository", "Repository Source"} {
		if err := page.Locator("text=" + text).First().WaitFor(
			playwright.LocatorWaitForOptions{Timeout: playwright.Float(gitQuickstartWaitMS)},
		); err != nil {
			t.Fatalf("wait for Git wizard text %q: %v\ndebug: %v", text, err, collectGitQuickstartDebug(page))
		}
	}
}

func clickGitWizardText(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	if err := page.Locator("text=" + text).First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(gitQuickstartWaitMS)},
	); err != nil {
		t.Fatalf("click Git wizard text %q: %v\ndebug: %v", text, err, collectGitQuickstartDebug(page))
	}
}

func waitForGitRepoViewerReady(t testing.TB, page playwright.Page, objectKey string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const objectKey = Array.isArray(arg) ? arg[0] : arg
		const timing =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		if (timing?.state === 'error') {
			throw new Error('Git quickstart failed: ' + (timing.error ?? 'unknown error'))
		}
		const hash = window.location.hash
		if (!hash.includes('/u/') || !hash.includes('/so/') || !hash.endsWith('/' + objectKey)) {
			return false
		}
		const text = document.body.textContent ?? ''
		if (text.includes('Git repository not found') || text.includes('Error loading repository')) {
			throw new Error(text.replace(/\s+/g, ' ').slice(0, 1000))
		}
		return text.includes('Empty Repository') &&
			text.includes(objectKey) &&
			text.includes('This repository has no commits yet.')
	}`, []any{objectKey}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(gitQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Git repo viewer %q: %v\ndebug: %v", objectKey, err, collectGitQuickstartDebug(page))
	}
}

func waitForGitWorktreeViewerReady(t testing.TB, page playwright.Page, objectKey string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const objectKey = Array.isArray(arg) ? arg[0] : arg
		const hash = window.location.hash
			if (!hash.includes('/u/') || !hash.includes('/so/') || !hash.endsWith('/' + objectKey)) {
				return false
			}
			const text = document.body.textContent ?? ''
			if (text.includes('Git worktree not found') || text.includes('Error loading worktree') || text.includes('Error loading files')) {
				throw new Error(text.replace(/\s+/g, ' ').slice(0, 1000))
			}
			return text.includes('Files') &&
				text.includes('Workdir') &&
				text.includes('Changes') &&
				text.includes(objectKey)
		}`, []any{objectKey}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(gitQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Git worktree viewer %q: %v\ndebug: %v", objectKey, err, collectGitQuickstartDebug(page))
	}
}

func assertGitEmptyWorktreeHidesLogTab(t testing.TB, page playwright.Page, objectKey string) {
	t.Helper()

	if count, err := page.Locator("button:has-text('Log')").Count(); err != nil {
		t.Fatalf("count Git worktree Log tabs: %v\ndebug: %v", err, collectGitQuickstartDebug(page))
	} else if count != 0 {
		t.Fatalf("expected empty Git worktree to hide Log tab, found %d\ndebug: %v", count, collectGitQuickstartDebug(page))
	}
	waitForGitWorktreeEmptyState(t, page, objectKey)
}

func waitForGitWorktreeEmptyState(t testing.TB, page playwright.Page, objectKey string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const objectKey = Array.isArray(arg) ? arg[0] : arg
		const text = document.body.textContent ?? ''
		return text.includes('Empty Repository') &&
			text.includes(objectKey) &&
			text.includes('This repository has no commits yet.')
	}`, []any{objectKey}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(gitQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for empty Git worktree state %q: %v\ndebug: %v", objectKey, err, collectGitQuickstartDebug(page))
	}
}

func assertGitRouteReloadAndReopen(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	hash string,
	assertReady func(),
) {
	t.Helper()

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload Git route: %v", err)
	}
	WaitForApp(t, page)
	assertReady()

	NavigateHash(t, h, page, "#/")
	NavigateHash(t, h, page, hash)
	assertReady()
}

func collectGitQuickstartDebug(page playwright.Page) any {
	debug, err := page.Evaluate(`() => JSON.stringify({
		url: window.location.href,
		hash: window.location.hash,
		timing: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
		startup: globalThis.__swBootStatus ?? null,
		startupMarks: globalThis.__swStartupMarks ?? [],
		appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 2000) ?? '',
		inputs: Array.from(document.querySelectorAll('input')).map((input) => ({
			placeholder: input.getAttribute('placeholder'),
			value: input.value,
			checked: input.checked,
		})),
		buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
			text: button.textContent?.replace(/\s+/g, ' ').trim().slice(0, 180) ?? '',
			ariaLabel: button.getAttribute('aria-label') ?? '',
			disabled: button.disabled,
		})),
		headings: Array.from(document.querySelectorAll('h1,h2,h3,[data-slot="loading-title"],[data-slot="loading-detail"]')).map((el) => ({
			tag: el.tagName,
			text: el.textContent?.replace(/\s+/g, ' ').trim().slice(0, 180) ?? '',
		})),
		bodyText: document.body.textContent?.replace(/\s+/g, ' ').slice(0, 2500) ?? '',
	})`)
	if err != nil {
		return "failed to collect Git quickstart debug: " + err.Error()
	}
	if s, ok := debug.(string); ok {
		return strings.TrimSpace(s)
	}
	return debug
}
