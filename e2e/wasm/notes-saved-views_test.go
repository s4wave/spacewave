//go:build !skip_e2e && !js

package wasm

import (
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestNotebookSavedViewsAcrossClients exercises browser Resource clients over
// the same retained SharedObject, including local drafts, reload, and deletion.
// Both pages share a Session, so its personal StateAtoms may synchronize.
func TestNotebookSavedViewsAcrossClients(t *testing.T) {
	h := harness(t)
	first := h.NewRetainedStatePageSession(t)
	firstPage := first.Page()
	scenario := createNotesDynamicQuickstartScenario(t, h, firstPage, "notebook")
	waitForNotebookReady(t, firstPage, "welcome")
	second := h.NewRetainedStatePageSession(t)
	secondPage := second.Page()
	NavigateHash(t, h, secondPage, scenario.objectHash("notebook"))
	if err := secondPage.Locator("input[placeholder='Search notes…']:visible").WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
	}); err != nil {
		body, _ := secondPage.Locator("body").InnerText()
		t.Fatalf("second Notebook: %v\nurl: %s\nbody: %s", err, secondPage.URL(), body)
	}

	panel := func(page playwright.Page) playwright.Locator {
		return page.Locator("details:visible").Filter(playwright.LocatorFilterOptions{
			Has: page.Locator("summary").Filter(playwright.LocatorFilterOptions{HasText: "Shared views"}),
		})
	}
	click := func(locator playwright.Locator) {
		t.Helper()
		if err := locator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(15000)}); err != nil {
			t.Fatal(err)
		}
	}
	button := func(scope playwright.Locator, name string) playwright.Locator {
		return scope.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: name, Exact: new(true)})
	}
	waitView := func(page playwright.Page, name string, state *playwright.WaitForSelectorState) {
		t.Helper()
		if err := panel(page).Locator("option").Filter(playwright.LocatorFilterOptions{HasText: name}).WaitFor(playwright.LocatorWaitForOptions{
			State: state, Timeout: playwright.Float(15000),
		}); err != nil {
			firstBody, _ := firstPage.Locator("body").InnerText()
			secondBody, _ := secondPage.Locator("body").InnerText()
			t.Fatalf("shared view %q: %v\nfirst: %s\nsecond: %s", name, err, firstBody, secondBody)
		}
	}
	for _, page := range []playwright.Page{firstPage, secondPage} {
		click(panel(page).Locator("summary"))
	}
	search := secondPage.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search notes", Exact: new(true)})
	if err := search.Fill("personal search"); err != nil {
		t.Fatal(err)
	}

	click(button(panel(firstPage), "Save new view"))
	if err := firstPage.GetByLabel("View name").Fill("Team review"); err != nil {
		t.Fatal(err)
	}
	click(firstPage.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save for everyone", Exact: new(true)}))
	waitView(secondPage, "Team review", playwright.WaitForSelectorStateAttached)
	if value, err := search.InputValue(); err != nil || value != "personal search" {
		t.Fatalf("shared save changed the other client's search: %q, %v", value, err)
	}

	click(button(panel(secondPage), "Save new view"))
	if err := secondPage.GetByLabel("View name").Fill("Unsubmitted draft"); err != nil {
		t.Fatal(err)
	}

	click(button(panel(firstPage), "Rename"))
	if err := firstPage.GetByLabel("View name").Fill("Team planning"); err != nil {
		t.Fatal(err)
	}
	click(firstPage.GetByRole("dialog").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Rename", Exact: new(true)}))
	waitView(secondPage, "Team planning", playwright.WaitForSelectorStateAttached)
	if value, err := secondPage.GetByLabel("View name").InputValue(); err != nil || value != "Unsubmitted draft" {
		t.Fatalf("shared rename changed the other client's draft: %q, %v", value, err)
	}
	click(secondPage.GetByRole("dialog").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: new(true)}))
	if _, err := secondPage.Reload(); err != nil {
		t.Fatal(err)
	}
	WaitForApp(t, secondPage)
	click(panel(secondPage).Locator("summary"))
	waitView(secondPage, "Team planning", playwright.WaitForSelectorStateAttached)

	labels := []string{"Team planning"}
	if _, err := panel(secondPage).GetByLabel("Saved view").SelectOption(playwright.SelectOptionValues{Labels: &labels}); err != nil {
		t.Fatal(err)
	}
	click(button(panel(firstPage), "Delete"))
	click(firstPage.GetByRole("dialog").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete view", Exact: new(true)}))
	waitView(secondPage, "Team planning", playwright.WaitForSelectorStateDetached)
	if err := panel(secondPage).GetByText("The selected view is no longer available.", playwright.LocatorGetByTextOptions{Exact: new(false)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
}
