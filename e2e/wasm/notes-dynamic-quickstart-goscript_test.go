//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// notesDynamicQuickstartWaitMS bounds every notes quickstart wait.
const notesDynamicQuickstartWaitMS = 30000

// notesDynamicQuickstartScenario records the space created by a notes
// dynamic quickstart.
type notesDynamicQuickstartScenario struct {
	sessionIndex uint32
	spaceID      string
}

// TestGoScriptNotesDynamicQuickstartsParity proves the notebook, docs, and
// blog dynamic quickstarts persist edits across route reloads under GoScript.
func TestGoScriptNotesDynamicQuickstartsParity(t *testing.T) {
	// Run the notebook quickstart and verify its edits survive a reload.
	t.Run("notebook", func(t *testing.T) {
		// Open a fresh session and run the notebook quickstart.
		sess := harness(t).NewCleanPageSession(t)
		page := sess.Page()
		scenario := createNotesDynamicQuickstartScenario(t, harness(t), page, "notebook")

		// Write a proof note through the notebook editor.
		waitForNotebookReady(t, page, "welcome")
		openNotebookNote(t, page, "welcome")
		writeSourceNote(t, page, strings.Join([]string{
			"---",
			"title: GoScript Notebook Proof",
			"tags: [goscript]",
			"---",
			"",
			"# GoScript Notebook Proof",
			"",
			"Notebook dynamic quickstart edits persist.",
			"",
		}, "\n"))
		assertNoteText(t, page, "Notebook dynamic quickstart edits persist.")

		// Reload and reopen the notebook note from the persisted object.
		assertNotesRouteReloadAndReopen(t, harness(t), page, scenario.objectHash("notebook"), func() {
			waitForNotebookReady(t, page, "GoScript Notebook Proof")
			openNotebookNote(t, page, "GoScript Notebook Proof")
			assertNoteText(t, page, "Notebook dynamic quickstart edits persist.")
		})
	})

	// Run the docs quickstart and verify its edits survive a reload.
	t.Run("docs", func(t *testing.T) {
		// Open a fresh session and run the docs quickstart.
		sess := harness(t).NewCleanPageSession(t)
		page := sess.Page()
		scenario := createNotesDynamicQuickstartScenario(t, harness(t), page, "docs")

		// Write a proof note through the docs editor.
		waitForDocsReady(t, page, "index")
		writeSourceNote(t, page, strings.Join([]string{
			"# GoScript Docs Proof",
			"",
			"Docs dynamic quickstart edits persist.",
			"",
		}, "\n"))
		assertNoteText(t, page, "Docs dynamic quickstart edits persist.")

		// Reload and reopen the docs page from the persisted object.
		assertNotesRouteReloadAndReopen(t, harness(t), page, scenario.objectHash("documentation"), func() {
			waitForDocsReady(t, page, "index")
			assertNoteText(t, page, "Docs dynamic quickstart edits persist.")
		})
	})

	// Run the blog quickstart and verify its edits survive a reload.
	t.Run("blog", func(t *testing.T) {
		// Open a fresh session and run the blog quickstart.
		sess := harness(t).NewCleanPageSession(t)
		page := sess.Page()
		scenario := createNotesDynamicQuickstartScenario(t, harness(t), page, "blog")

		// Switch the blog to editing mode and open the starter post.
		waitForBlogReady(t, page, "Hello World")
		if err := page.Locator("button[title='Editing mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to editing mode: %v", err)
		}
		openNotebookNote(t, page, "Hello World")

		// Rewrite the blog post source with the proof content.
		writeSourceNote(t, page, strings.Join([]string{
			"---",
			"title: GoScript Blog Proof",
			"date: 2026-04-17",
			"author: writer",
			"summary: Dynamic quickstart blog proof.",
			"tags: [goscript]",
			"draft: false",
			"---",
			"",
			"# GoScript Blog Proof",
			"",
			"Blog dynamic quickstart edits persist.",
			"",
		}, "\n"))

		// Navigate to the blog site object and open the edited post.
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "GoScript Blog Proof")
		if err := page.Locator("text=GoScript Blog Proof").First().Click(); err != nil {
			t.Fatalf("open edited blog post: %v", err)
		}
		assertNoteText(t, page, "Blog dynamic quickstart edits persist.")

		// Reload and reopen the edited blog post from the persisted object.
		assertNotesRouteReloadAndReopen(t, harness(t), page, scenario.objectHash("blog/site"), func() {
			waitForBlogReady(t, page, "GoScript Blog Proof")
			if err := page.Locator("text=GoScript Blog Proof").First().Click(); err != nil {
				t.Fatalf("reopen edited blog post: %v", err)
			}
			assertNoteText(t, page, "Blog dynamic quickstart edits persist.")
		})
	})
}

// TestGoScriptBlogDynamicQuickstartRetainedStateParity proves the blog
// quickstart route reloads under a retained-state browser session.
func TestGoScriptBlogDynamicQuickstartRetainedStateParity(t *testing.T) {
	// Run the blog quickstart in a retained-state session and reopen its route.
	sess := harness(t).NewRetainedStatePageSession(t)
	page := sess.Page()
	scenario := createNotesDynamicQuickstartScenario(t, harness(t), page, "blog")
	waitForBlogReady(t, page, "Hello World")
	NavigateHash(t, harness(t), page, scenario.objectHash("blog/site"))
	waitForBlogReady(t, page, "Hello World")
}

// createNotesDynamicQuickstartScenario runs a notes quickstart and records its
// created space identity.
func createNotesDynamicQuickstartScenario(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	quickstartID string,
) *notesDynamicQuickstartScenario {
	// Mark the scenario helper as a test helper.
	t.Helper()

	// Drive the browser through the quickstart until its space route loads.
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/"+quickstartID)
	waitForNotesQuickstartSpaceRoute(t, page, quickstartID)

	// Parse the created space's identity out of the quickstart route.
	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse %s quickstart route: %v", quickstartID, err)
	}
	return &notesDynamicQuickstartScenario{
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

// objectHash returns the session object route hash for the given object key.
func (s *notesDynamicQuickstartScenario) objectHash(objectKey string) string {
	return fmt.Sprintf("#/u/%d/so/%s/-/%s", s.sessionIndex, s.spaceID, objectKey)
}

// waitForNotesQuickstartSpaceRoute waits for the quickstart's space route to
// load, failing with collected debug state.
func waitForNotesQuickstartSpaceRoute(t testing.TB, page playwright.Page, quickstartID string) {
	t.Helper()

	// The docs quickstart shares the notes app and needs no route probe.
	if quickstartID == "docs" {
		waitForDocsReady(t, page, "index")
		return
	}

	// Probe the page until the quickstart space route renders its UI.
	_, err := page.WaitForFunction(`(arg) => {
		const quickstartID = Array.isArray(arg) ? arg[0] : arg
		const timing =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		if (timing?.state === 'error') {
			throw new Error(
				quickstartID + ' quickstart failed: ' + (timing.error ?? 'unknown error'),
			)
		}
		if (!window.location.hash.includes('/u/') || !window.location.hash.includes('/so/')) {
			return false
		}
		if (quickstartID === 'blog') {
			return document.querySelector("button[title='Reading mode']") !== null
		}
		return document.querySelector("input[placeholder='Search notes…']") !== null
	}`, []any{quickstartID}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(notesDynamicQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for %s quickstart route: %v\ndebug: %v", quickstartID, err, collectNotesDynamicQuickstartDebug(page))
	}
}

// waitForDocsReady waits for the docs sidebar, page entry, and content view.
func waitForDocsReady(t testing.TB, page playwright.Page, pageName string) {
	// Mark the docs helper as a test helper.
	t.Helper()

	// Wait for the docs pages sidebar.
	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(notesDynamicQuickstartWaitMS)}
	if err := page.Locator("text=Pages").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for docs pages sidebar: %v\ndebug: %v", err, collectNotesDynamicQuickstartDebug(page))
	}

	// Wait for the named docs page entry when one is requested.
	if pageName != "" {
		if err := page.Locator("text=" + pageName).First().WaitFor(wait); err != nil {
			t.Fatalf("wait for docs page %q: %v\ndebug: %v", pageName, err, collectNotesDynamicQuickstartDebug(page))
		}
	}

	// Wait for the docs content view to render.
	if err := page.Locator("[data-testid='notes-content-view']").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for docs content view: %v\ndebug: %v", err, collectNotesDynamicQuickstartDebug(page))
	}
}

// assertNoteText waits until the given text renders in the notes app.
func assertNoteText(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	// Wait for the text locator to appear.
	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(notesDynamicQuickstartWaitMS)}
	if err := page.Locator("text=" + text).First().WaitFor(wait); err != nil {
		t.Fatalf("wait for note text %q: %v\ndebug: %v", text, err, collectNotesDynamicQuickstartDebug(page))
	}
}

// assertNotesRouteReloadAndReopen reloads the route and reopens it from the
// app root, asserting readiness each time.
func assertNotesRouteReloadAndReopen(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	hash string,
	assertReady func(),
) {
	// Mark the reload helper as a test helper.
	t.Helper()

	// Reload the page and assert the route rehydrates.
	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload notes route: %v", err)
	}
	WaitForApp(t, page)
	assertReady()

	// Navigate away and back to prove the route reopens from the root.
	NavigateHash(t, h, page, "#/")
	NavigateHash(t, h, page, hash)
	assertReady()
}

// collectNotesDynamicQuickstartDebug gathers page state for failure messages.
func collectNotesDynamicQuickstartDebug(page playwright.Page) any {
	// Evaluate a debug snapshot of the page's URL, timing, and DOM.
	debug, err := page.Evaluate(`() => JSON.stringify({
		url: window.location.href,
		hash: window.location.hash,
		timing: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
		appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 1800) ?? '',
		inputs: Array.from(document.querySelectorAll('input')).map((input) => ({
			placeholder: input.getAttribute('placeholder'),
			value: input.value,
		})),
		buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
			title: button.getAttribute('title'),
			testid: button.getAttribute('data-testid'),
			text: button.textContent?.replace(/\s+/g, ' ').slice(0, 80) ?? '',
		})),
		testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
			testid: el.getAttribute('data-testid'),
			text: el.textContent?.replace(/\s+/g, ' ').slice(0, 160) ?? '',
		})),
	})`)
	if err != nil {
		return "failed to collect notes dynamic quickstart debug: " + err.Error()
	}
	return debug
}
