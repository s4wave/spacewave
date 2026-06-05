//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const notesDynamicQuickstartWaitMS = 240000

type notesDynamicQuickstartScenario struct {
	sessionIndex uint32
	spaceID      string
}

func TestGoScriptNotesDynamicQuickstartsParity(t *testing.T) {
	t.Run("notebook", func(t *testing.T) {
		sess := testHarness.NewCleanPageSession(t)
		page := sess.Page()
		scenario := createNotesDynamicQuickstartScenario(t, testHarness, page, "notebook")
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
		assertNotesRouteReloadAndReopen(t, testHarness, page, scenario.objectHash("notebook"), func() {
			waitForNotebookReady(t, page, "GoScript Notebook Proof")
			openNotebookNote(t, page, "GoScript Notebook Proof")
			assertNoteText(t, page, "Notebook dynamic quickstart edits persist.")
		})
	})

	t.Run("docs", func(t *testing.T) {
		sess := testHarness.NewCleanPageSession(t)
		page := sess.Page()
		scenario := createNotesDynamicQuickstartScenario(t, testHarness, page, "docs")
		waitForDocsReady(t, page, "index")
		writeSourceNote(t, page, strings.Join([]string{
			"# GoScript Docs Proof",
			"",
			"Docs dynamic quickstart edits persist.",
			"",
		}, "\n"))
		assertNoteText(t, page, "Docs dynamic quickstart edits persist.")
		assertNotesRouteReloadAndReopen(t, testHarness, page, scenario.objectHash("documentation"), func() {
			waitForDocsReady(t, page, "index")
			assertNoteText(t, page, "Docs dynamic quickstart edits persist.")
		})
	})

	t.Run("blog", func(t *testing.T) {
		sess := testHarness.NewCleanPageSession(t)
		page := sess.Page()
		scenario := createNotesDynamicQuickstartScenario(t, testHarness, page, "blog")
		waitForBlogReady(t, page, "Hello World")
		if err := page.Locator("button[title='Editing mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to editing mode: %v", err)
		}
		openNotebookNote(t, page, "Hello World")
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
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "GoScript Blog Proof")
		if err := page.Locator("text=GoScript Blog Proof").First().Click(); err != nil {
			t.Fatalf("open edited blog post: %v", err)
		}
		assertNoteText(t, page, "Blog dynamic quickstart edits persist.")
		assertNotesRouteReloadAndReopen(t, testHarness, page, scenario.objectHash("blog/site"), func() {
			waitForBlogReady(t, page, "GoScript Blog Proof")
			if err := page.Locator("text=GoScript Blog Proof").First().Click(); err != nil {
				t.Fatalf("reopen edited blog post: %v", err)
			}
			assertNoteText(t, page, "Blog dynamic quickstart edits persist.")
		})
	})
}

func TestGoScriptBlogDynamicQuickstartRetainedStateParity(t *testing.T) {
	sess := testHarness.NewRetainedStatePageSession(t)
	page := sess.Page()
	scenario := createNotesDynamicQuickstartScenario(t, testHarness, page, "blog")
	waitForBlogReady(t, page, "Hello World")
	NavigateHash(t, testHarness, page, scenario.objectHash("blog/site"))
	waitForBlogReady(t, page, "Hello World")
}

func createNotesDynamicQuickstartScenario(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	quickstartID string,
) *notesDynamicQuickstartScenario {
	t.Helper()

	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/"+quickstartID)
	waitForNotesQuickstartSpaceRoute(t, page, quickstartID)

	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse %s quickstart route: %v", quickstartID, err)
	}
	return &notesDynamicQuickstartScenario{
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

func (s *notesDynamicQuickstartScenario) objectHash(objectKey string) string {
	return fmt.Sprintf("#/u/%d/so/%s/-/%s", s.sessionIndex, s.spaceID, objectKey)
}

func waitForNotesQuickstartSpaceRoute(t testing.TB, page playwright.Page, quickstartID string) {
	t.Helper()

	if quickstartID == "docs" {
		waitForDocsReady(t, page, "index")
		return
	}

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
		return document.querySelector("input[placeholder='Search notes...']") !== null
	}`, []any{quickstartID}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(notesDynamicQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for %s quickstart route: %v\ndebug: %v", quickstartID, err, collectNotesDynamicQuickstartDebug(page))
	}
}

func waitForDocsReady(t testing.TB, page playwright.Page, pageName string) {
	t.Helper()

	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(notesDynamicQuickstartWaitMS)}
	if err := page.Locator("text=Pages").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for docs pages sidebar: %v\ndebug: %v", err, collectNotesDynamicQuickstartDebug(page))
	}
	if pageName != "" {
		if err := page.Locator("text=" + pageName).First().WaitFor(wait); err != nil {
			t.Fatalf("wait for docs page %q: %v\ndebug: %v", pageName, err, collectNotesDynamicQuickstartDebug(page))
		}
	}
	if err := page.Locator("[data-testid='notes-content-view']").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for docs content view: %v\ndebug: %v", err, collectNotesDynamicQuickstartDebug(page))
	}
}

func assertNoteText(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(notesDynamicQuickstartWaitMS)}
	if err := page.Locator("text=" + text).First().WaitFor(wait); err != nil {
		t.Fatalf("wait for note text %q: %v\ndebug: %v", text, err, collectNotesDynamicQuickstartDebug(page))
	}
}

func assertNotesRouteReloadAndReopen(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	hash string,
	assertReady func(),
) {
	t.Helper()

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload notes route: %v", err)
	}
	WaitForApp(t, page)
	assertReady()

	NavigateHash(t, h, page, "#/")
	NavigateHash(t, h, page, hash)
	assertReady()
}

func collectNotesDynamicQuickstartDebug(page playwright.Page) any {
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
