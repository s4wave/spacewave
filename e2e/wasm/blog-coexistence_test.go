//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

const blogCoexistenceWaitMS = 240000

type blogScenario struct {
	session      *TestSession
	sessionIndex uint32
	spaceID      string
}

func createBlogScenario(t testing.TB, h *Harness, session *TestSession) *blogScenario {
	// Load the blog quickstart page and wait for it to boot.
	t.Helper()

	// Prepare the page and enable quickstart timing logs.
	page := session.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	t.Log("navigate to blog quickstart")
	NavigateHash(t, h, page, "#/quickstart/blog")
	_, err := page.Evaluate(h.Script("wait-for-blog.ts"), map[string]any{
		"deadlineMs": blogCoexistenceWaitMS,
	})
	if err != nil {
		t.Fatalf("wait for blog quickstart: %v", err)
	}
	t.Logf("blog quickstart route: %s", page.URL())

	// Wait for the blog reading view to render the seeded post.
	waitForBlogReady(t, page, "Hello World")

	// Parse the blog quickstart route into a scenario.
	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse blog route: %v", err)
	}

	return &blogScenario{
		session:      session,
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

func (s *blogScenario) objectHash(objectKey string) string {
	return fmt.Sprintf("#/u/%d/so/%s/-/%s", s.sessionIndex, s.spaceID, objectKey)
}

func waitForBlogReady(t testing.TB, page playwright.Page, title string) {
	// Format the object route hash for the scenario.
	t.Helper()

	// Wait for the blog reading and editing buttons to appear.
	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(blogCoexistenceWaitMS)}
	if err := page.Locator("button[title='Reading mode']").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for blog reading button: %v", err)
	}
	if err := page.Locator("button[title='Editing mode']").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for blog editing button: %v", err)
	}
	if title == "" {
		return
	}
	if err := page.Locator("text=" + title).First().WaitFor(wait); err != nil {
		body, bodyErr := page.Locator("body").InnerText()
		if bodyErr != nil {
			body = "failed to read body text: " + bodyErr.Error()
		}
		debug, debugErr := page.Evaluate(`() => JSON.stringify({
			url: window.location.href,
			hash: window.location.hash,
			hasDebugRoot: !!globalThis.__s4wave_debug?.root,
			quickstartTiming: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
			appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 1600) ?? '',
			buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
				title: button.getAttribute('title'),
				text: button.textContent?.replace(/\s+/g, ' ').slice(0, 80) ?? '',
			})),
			testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
				testid: el.getAttribute('data-testid'),
				text: el.textContent?.replace(/\s+/g, ' ').slice(0, 120) ?? '',
			})),
		})`)
		if debugErr != nil {
			debug = "failed to collect blog debug: " + debugErr.Error()
		}
		t.Fatalf("wait for blog title %q: %v\nbody: %s\ndebug: %v", title, err, trimPageText(body), debug)
	}
}

func waitForNotebookReady(t testing.TB, page playwright.Page, noteTitle string) {
	// Wait for the notebook search input to appear.
	t.Helper()

	// Collect notebook debug state when the search input never appears.
	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(blogCoexistenceWaitMS)}
	if err := page.Locator("input[placeholder='Search notes…']").First().WaitFor(wait); err != nil {
		debug, debugErr := page.Evaluate(`() => JSON.stringify({
			url: window.location.href,
			hash: window.location.hash,
			appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 1600) ?? '',
			inputs: Array.from(document.querySelectorAll('input')).map((input) => ({
				placeholder: input.getAttribute('placeholder'),
				value: input.value,
			})),
			buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
				title: button.getAttribute('title'),
				text: button.textContent?.replace(/\s+/g, ' ').slice(0, 80) ?? '',
			})),
		})`)
		if debugErr != nil {
			debug = "failed to collect notebook debug: " + debugErr.Error()
		}
		t.Fatalf("wait for notebook search: %v\ndebug: %v", err, debug)
	}
	if noteTitle == "" {
		return
	}
	if err := page.Locator("text=" + noteTitle).First().WaitFor(wait); err != nil {
		t.Fatalf("wait for notebook note %q: %v\ndebug: %v", noteTitle, err, collectNotebookDebug(page))
	}
}

func openNotebookNote(t testing.TB, page playwright.Page, noteTitle string) {
	// Wait for the notebook row and open the note.
	t.Helper()

	// Locate the note row within the wait deadline.
	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(blogCoexistenceWaitMS)}
	row := page.Locator("[data-testid='notes-note-row']:has-text('" + noteTitle + "')").First()
	if err := row.WaitFor(wait); err != nil {
		t.Fatalf("wait for notebook row %q: %v", noteTitle, err)
	}
	if err := row.Click(); err != nil {
		t.Fatalf("click notebook row %q: %v", noteTitle, err)
	}
	if err := page.Locator("[data-testid='notes-content-view']").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for notebook content view %q: %v\ndebug: %v", noteTitle, err, collectNotebookDebug(page))
	}
}

func writeSourceNote(t testing.TB, page playwright.Page, content string) {
	// Switch the note to source mode and fill its content.
	t.Helper()

	// Click the source toggle and wait for the editor.
	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(blogCoexistenceWaitMS)}
	sourceBtn := page.Locator("[data-testid='notes-source-toggle'][title='Switch to source']").First()
	if err := sourceBtn.WaitFor(wait); err != nil {
		t.Fatalf("wait for source button: %v\ndebug: %v", err, collectNotebookDebug(page))
	}
	if err := sourceBtn.Click(); err != nil {
		t.Fatalf("click source button: %v", err)
	}

	// Fill the source editor with the note content.
	editor := page.Locator("textarea").First()
	if err := editor.WaitFor(wait); err != nil {
		t.Fatalf("wait for source editor: %v", err)
	}
	if err := editor.Fill(content); err != nil {
		t.Fatalf("fill source editor: %v", err)
	}

	// Switch back to WYSIWYG mode to save the note.
	saveBtn := page.Locator("[data-testid='notes-source-toggle'][title='Switch to WYSIWYG']").First()
	if err := saveBtn.Click(); err != nil {
		t.Fatalf("click WYSIWYG button: %v", err)
	}
	if err := sourceBtn.WaitFor(wait); err != nil {
		t.Fatalf("wait for source edit save to settle: %v\ndebug: %v", err, collectNotebookDebug(page))
	}
}

func collectNotebookDebug(page playwright.Page) any {
	debug, err := page.Evaluate(`() => JSON.stringify({
		url: window.location.href,
		hash: window.location.hash,
		appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 1600) ?? '',
		activeElement: document.activeElement ? {
			tag: document.activeElement.tagName,
			text: document.activeElement.textContent?.replace(/\s+/g, ' ').slice(0, 120) ?? '',
			testid: document.activeElement.getAttribute('data-testid'),
		} : null,
		rows: Array.from(document.querySelectorAll('[data-testid="notes-note-row"]')).map((row) => {
			const rect = row.getBoundingClientRect()
			return {
				text: row.textContent?.replace(/\s+/g, ' ').slice(0, 120) ?? '',
				path: row.getAttribute('data-note-path'),
				visible: rect.width > 0 && rect.height > 0,
				rect: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
				className: row.getAttribute('class'),
			}
		}),
		contentViews: Array.from(document.querySelectorAll('[data-testid="notes-content-view"]')).map((view) => {
			const rect = view.getBoundingClientRect()
			return {
				text: view.textContent?.replace(/\s+/g, ' ').slice(0, 300) ?? '',
				visible: rect.width > 0 && rect.height > 0,
				rect: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
			}
		}),
		buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
			title: button.getAttribute('title'),
			testid: button.getAttribute('data-testid'),
			text: button.textContent?.replace(/\s+/g, ' ').slice(0, 80) ?? '',
		})),
	})`)
	if err != nil {
		return "failed to collect notebook debug: " + err.Error()
	}
	return debug
}

func TestBlogCoexistenceScenario(t *testing.T) {
	// Start a clean page session and create the blog scenario.
	sess := harness(t).NewCleanPageSession(t)
	scenario := createBlogScenario(t, harness(t), sess)
	page := sess.Page()

	// Open the companion notebook and edit the hello world note.
	t.Run("notebook edits appear in blog reading view", func(t *testing.T) {
		// Navigate to the companion notebook.
		t.Log("open companion notebook")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site-notebook"))
		waitForNotebookReady(t, page, "Hello World")
		t.Log("open hello world note")
		openNotebookNote(t, page, "Hello World")

		// Rewrite the hello world note in source mode.
		t.Log("edit hello world note in source mode")
		writeSourceNote(t, page, strings.Join([]string{
			"---",
			"title: Shared Update",
			"date: 2026-04-17",
			"author: writer",
			"summary: Updated from the companion notebook.",
			"tags: [sync]",
			"draft: false",
			"---",
			"",
			"# Shared Update",
			"",
			"Notebook edits reach the blog reading view.",
			"",
		}, "\n"))

		// Return to the blog reader and verify the updated post.
		t.Log("return to blog reader")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "Shared Update")
		if err := page.Locator("text=Shared Update").First().Click(); err != nil {
			t.Fatalf("open updated blog post: %v", err)
		}
		if err := page.Locator("text=Notebook edits reach the blog reading view.").First().WaitFor(); err != nil {
			t.Fatalf("wait for updated blog body: %v", err)
		}
	})

	// Create a new post from blog editing mode and verify it in the notebook.
	t.Run("blog editor creates published post visible in notebook", func(t *testing.T) {
		// Open the blog viewer.
		t.Log("open blog viewer")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "Shared Update")

		// Create a new post and write it from the blog editor.
		t.Log("create new post from blog editing mode")
		if err := page.Locator("button[title='Editing mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to editing mode: %v", err)
		}
		wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(blogCoexistenceWaitMS)}
		newPostBtn := page.Locator("button[title='New note']").First()
		if err := newPostBtn.WaitFor(wait); err != nil {
			t.Fatalf("wait for new post button: %v", err)
		}
		if err := newPostBtn.Click(); err != nil {
			t.Fatalf("click new post button: %v", err)
		}
		if err := page.Locator("[data-testid='notes-content-view']:has-text('New Post')").First().WaitFor(wait); err != nil {
			t.Fatalf("wait for new post editor: %v\ndebug: %v", err, collectNotebookDebug(page))
		}

		// Write the second post in source mode.
		writeSourceNote(t, page, strings.Join([]string{
			"---",
			"title: Second Post",
			"date: 2026-04-18",
			"author: editor",
			"summary: Created from blog editing mode.",
			"tags: [coexistence]",
			"draft: false",
			"---",
			"",
			"# Second Post",
			"",
			"Created in the blog editor.",
			"",
		}, "\n"))
		if err := page.Locator("[data-testid='notes-content-view']:has-text('Second Post')").First().WaitFor(wait); err != nil {
			t.Fatalf("wait for edited post content: %v\ndebug: %v", err, collectNotebookDebug(page))
		}

		// Wait for the published post in the blog reader.
		t.Log("wait for published post in blog reader")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "Second Post")

		// Verify the published post appears in the notebook.
		t.Log("verify published post appears in notebook")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site-notebook"))
		waitForNotebookReady(t, page, "Second Post")
	})

	// Verify plain notebook notes stay out of the blog reading view.
	t.Run("non-blog files stay out of reading view but appear in blog editing mode", func(t *testing.T) {
		// Create a plain notebook note.
		t.Log("create plain notebook note")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site-notebook"))
		waitForNotebookReady(t, page, "")

		// Create a new untitled note in the notebook.
		newNoteBtn := page.Locator("button[title='New note']").First()
		if err := newNoteBtn.Click(); err != nil {
			t.Fatalf("click notebook new note button: %v", err)
		}
		waitForNotebookReady(t, page, "untitled")

		// Verify the plain note is hidden in reading mode.
		t.Log("verify plain note hidden in reading mode")
		NavigateHash(t, harness(t), page, scenario.objectHash("blog/site"))
		if err := page.Locator("button[title='Reading mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to reading mode for plain note check: %v", err)
		}
		backToPosts := page.Locator("button:has-text('Back to posts')").First()
		backToPostsCount, err := backToPosts.Count()
		if err != nil {
			t.Fatalf("count back to posts button: %v", err)
		}
		if backToPostsCount > 0 {
			if err := backToPosts.Click(); err != nil {
				t.Fatalf("return to blog post index: %v", err)
			}
		}
		waitForBlogReady(t, page, "Second Post")

		// Count untitled entries in the blog reading view.
		count, err := page.Locator("article h3:has-text('untitled')").Count()
		if err != nil {
			t.Fatalf("count untitled entries in reading mode: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected untitled note to be hidden in reading mode, found %d match(es)", count)
		}

		// Verify the plain note appears in blog editing mode.
		if err := page.Locator("button[title='Editing mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to editing mode for note list: %v", err)
		}
		if err := page.Locator("text=untitled").First().WaitFor(); err != nil {
			t.Fatalf("wait for untitled note in blog editing mode: %v", err)
		}
	})
}
