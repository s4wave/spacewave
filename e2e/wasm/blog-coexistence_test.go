//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

type blogScenario struct {
	session      *TestSession
	sessionIndex uint32
	spaceID      string
}

func createBlogScenario(t testing.TB, h *Harness, session *TestSession) *blogScenario {
	t.Helper()

	page := session.Page()
	WaitForApp(t, page)
	t.Log("navigate to blog quickstart")
	NavigateHash(t, h, page, "#/quickstart/blog")
	_, err := page.Evaluate(h.Script("wait-for-blog.ts"), map[string]any{
		"deadlineMs": 120000,
	})
	if err != nil {
		t.Fatalf("wait for blog quickstart: %v", err)
	}
	t.Logf("blog quickstart route: %s", page.URL())
	waitForBlogReady(t, page, "Hello World")

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
	t.Helper()

	if err := page.Locator("button[title='Reading mode']").First().WaitFor(); err != nil {
		t.Fatalf("wait for blog reading button: %v", err)
	}
	if err := page.Locator("button[title='Editing mode']").First().WaitFor(); err != nil {
		t.Fatalf("wait for blog editing button: %v", err)
	}
	if title == "" {
		return
	}
	if err := page.Locator("text=" + title).First().WaitFor(); err != nil {
		t.Fatalf("wait for blog title %q: %v", title, err)
	}
}

func waitForNotebookReady(t testing.TB, page playwright.Page, noteTitle string) {
	t.Helper()

	if err := page.Locator("input[placeholder='Search notes...']").First().WaitFor(); err != nil {
		t.Fatalf("wait for notebook search: %v", err)
	}
	if noteTitle == "" {
		return
	}
	if err := page.Locator("text=" + noteTitle).First().WaitFor(); err != nil {
		t.Fatalf("wait for notebook note %q: %v", noteTitle, err)
	}
}

func openNotebookNote(t testing.TB, page playwright.Page, noteTitle string) {
	t.Helper()

	row := page.Locator("text=" + noteTitle).First()
	if err := row.WaitFor(); err != nil {
		t.Fatalf("wait for notebook row %q: %v", noteTitle, err)
	}
	if err := row.Click(); err != nil {
		t.Fatalf("click notebook row %q: %v", noteTitle, err)
	}
}

func writeSourceNote(t testing.TB, page playwright.Page, content string) {
	t.Helper()

	sourceBtn := page.Locator("button:has-text('Source')").First()
	if err := sourceBtn.WaitFor(); err != nil {
		t.Fatalf("wait for source button: %v", err)
	}
	if err := sourceBtn.Click(); err != nil {
		t.Fatalf("click source button: %v", err)
	}

	editor := page.Locator("textarea").First()
	if err := editor.WaitFor(); err != nil {
		t.Fatalf("wait for source editor: %v", err)
	}
	if err := editor.Fill(content); err != nil {
		t.Fatalf("fill source editor: %v", err)
	}

	saveBtn := page.Locator("button:has-text('WYSIWYG')").First()
	if err := saveBtn.Click(); err != nil {
		t.Fatalf("click WYSIWYG button: %v", err)
	}
}

func TestBlogCoexistenceScenario(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := createBlogScenario(t, testHarness, sess)
	page := sess.Page()

	t.Run("notebook edits appear in blog reading view", func(t *testing.T) {
		t.Log("open companion notebook")
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site-notebook"))
		waitForNotebookReady(t, page, "Hello World")
		t.Log("open hello world note")
		openNotebookNote(t, page, "Hello World")

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

		t.Log("return to blog reader")
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "Shared Update")
		if err := page.Locator("text=Notebook edits reach the blog reading view.").First().WaitFor(); err != nil {
			t.Fatalf("wait for updated blog body: %v", err)
		}
	})

	t.Run("blog editor creates published post visible in notebook", func(t *testing.T) {
		t.Log("open blog viewer")
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "Shared Update")

		t.Log("create new post from blog editing mode")
		if err := page.Locator("button[title='Editing mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to editing mode: %v", err)
		}
		newPostBtn := page.Locator("button[title='New note']").First()
		if err := newPostBtn.WaitFor(); err != nil {
			t.Fatalf("wait for new post button: %v", err)
		}
		if err := newPostBtn.Click(); err != nil {
			t.Fatalf("click new post button: %v", err)
		}

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

		t.Log("verify published post appears in notebook")
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site-notebook"))
		waitForNotebookReady(t, page, "Second Post")
	})

	t.Run("non-blog files stay out of reading view but appear in blog editing mode", func(t *testing.T) {
		t.Log("create plain notebook note")
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site-notebook"))
		waitForNotebookReady(t, page, "Shared Update")

		newNoteBtn := page.Locator("button[title='New note']").First()
		if err := newNoteBtn.Click(); err != nil {
			t.Fatalf("click notebook new note button: %v", err)
		}
		waitForNotebookReady(t, page, "untitled")

		t.Log("verify plain note hidden in reading mode")
		NavigateHash(t, testHarness, page, scenario.objectHash("blog/site"))
		waitForBlogReady(t, page, "Shared Update")

		count, err := page.Locator("text=untitled").Count()
		if err != nil {
			t.Fatalf("count untitled entries in reading mode: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected untitled note to be hidden in reading mode, found %d match(es)", count)
		}

		if err := page.Locator("button[title='Editing mode']").First().Click(); err != nil {
			t.Fatalf("switch blog to editing mode for note list: %v", err)
		}
		if err := page.Locator("text=untitled").First().WaitFor(); err != nil {
			t.Fatalf("wait for untitled note in blog editing mode: %v", err)
		}
	})
}
