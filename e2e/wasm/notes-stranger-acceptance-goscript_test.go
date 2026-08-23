//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"testing"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

const notesStrangerMarker = "A-notes-local-001"

type notesShellDocumentState struct {
	Incarnation string `json:"incarnation"`
	ActiveTabID string `json:"activeTabId"`
}

func (s notesShellDocumentState) tabStateKey() string {
	return fmt.Sprintf("shell-tab-state:%s:%s", s.Incarnation, s.ActiveTabID)
}

func TestGoScriptNotesStrangerColdPageReopen(t *testing.T) {
	h := harness(t)
	sess := h.NewCleanPageSession(t)
	page := sess.Page()
	scenario := createNotesDynamicQuickstartScenario(t, h, page, "notebook")

	waitForNotebookReady(t, page, "welcome")
	openNotebookNote(t, page, "welcome")
	writeSourceNote(t, page, "# Welcome\n\nStart capturing notes in this Spacewave notebook.\n\n"+notesStrangerMarker+"\n")
	assertNoteText(t, page, notesStrangerMarker)

	oldDocument := readNotesShellDocumentState(t, page)
	oldTabStateKey := oldDocument.tabStateKey()
	assertSessionStorageKeyPresent(t, page, oldTabStateKey)

	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace Notes page: %v", err)
	}
	if err := sess.LoadApp(); err != nil {
		t.Fatalf("load Notes app after page replacement: %v", err)
	}
	page = sess.Page()
	WaitForApp(t, page)

	newDocument := readNotesShellDocumentState(t, page)
	if newDocument.Incarnation == oldDocument.Incarnation {
		t.Fatalf("fresh Notes page reused document incarnation %q", newDocument.Incarnation)
	}
	assertSessionStorageKeyAbsent(t, page, oldTabStateKey)

	NavigateHash(t, h, page, scenario.objectHash("notebook"))
	waitForNotebookReady(t, page, "welcome")
	openNotebookNote(t, page, "welcome")
	assertNoteText(t, page, "Start capturing notes in this Spacewave notebook.")
	assertNoteText(t, page, notesStrangerMarker)
}

func readNotesShellDocumentState(t testing.TB, page playwright.Page) notesShellDocumentState {
	t.Helper()

	_, err := page.WaitForFunction(`() => {
		const raw = sessionStorage.getItem('shell-document-state')
		if (!raw) return false
		try {
			const state = JSON.parse(raw)
			return typeof state.incarnation === 'string' && state.incarnation.length > 0 &&
				typeof state.activeTabId === 'string' && state.activeTabId.length > 0
		} catch {
			return false
		}
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(notesDynamicQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Shell document state: %v", err)
	}

	raw, err := page.Evaluate(`() => sessionStorage.getItem('shell-document-state')`)
	if err != nil {
		t.Fatalf("read Shell document state: %v", err)
	}
	serialized, ok := raw.(string)
	if !ok {
		t.Fatalf("Shell document state has type %T, want string", raw)
	}
	var parser fastjson.Parser
	v, err := parser.Parse(serialized)
	if err != nil {
		t.Fatalf("decode Shell document state: %v", err)
	}
	return notesShellDocumentState{
		Incarnation: string(v.GetStringBytes("incarnation")),
		ActiveTabID: string(v.GetStringBytes("activeTabId")),
	}
}

func assertSessionStorageKeyPresent(t testing.TB, page playwright.Page, key string) {
	t.Helper()

	value, err := page.Evaluate(`(key) => sessionStorage.getItem(key)`, key)
	if err != nil {
		t.Fatalf("read sessionStorage key %q: %v", key, err)
	}
	if value == nil {
		t.Fatalf("sessionStorage key %q is absent before page replacement", key)
	}
}

func assertSessionStorageKeyAbsent(t testing.TB, page playwright.Page, key string) {
	t.Helper()

	value, err := page.Evaluate(`(key) => sessionStorage.getItem(key)`, key)
	if err != nil {
		t.Fatalf("read sessionStorage key %q: %v", key, err)
	}
	if value != nil {
		t.Fatalf("old sessionStorage key %q survived page replacement", key)
	}
}
