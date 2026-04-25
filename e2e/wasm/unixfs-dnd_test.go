//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"
)

func waitForDriveBody(t testing.TB, pageText func() (string, error)) string {
	t.Helper()

	body, err := pageText()
	if err != nil {
		t.Fatalf("read drive browser text: %v", err)
	}
	return strings.TrimSpace(body)
}

func openDriveDir(t testing.TB, pageText func() (string, error), open func(name string), name string) {
	t.Helper()

	open(name)
	body := waitForDriveBody(t, pageText)
	if strings.Contains(body, "Loading...") {
		t.Fatalf("expected directory %q to finish loading, got %q", name, body)
	}
}

// TestQuickstartDriveSingleEntryRowMove traces the live same-viewer row-to-
// folder move path to classify whether the current report is stale or a real
// runtime regression against the implemented same-root move contract.
func TestQuickstartDriveSingleEntryRowMove(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()
	browser := page.Locator("[data-testid='unixfs-browser']")

	WaitForDriveReady(t, testHarness, page)
	t.Log("drive ready")

	bodyText := func() (string, error) {
		return browser.TextContent()
	}
	openDir := func(name string) {
		t.Helper()

		row := page.Locator("[role='row']").Locator("text=" + name).First()
		if err := row.WaitFor(); err != nil {
			t.Fatalf("wait for %s row: %v", name, err)
		}
		if err := row.Dblclick(); err != nil {
			t.Fatalf("open %s row: %v", name, err)
		}
	}

	source := page.Locator("[role='row']").Locator("text=hello.txt").First()
	if err := source.WaitFor(); err != nil {
		t.Fatalf("wait for source row: %v", err)
	}
	t.Log("source row ready")

	target := page.Locator("[role='row']").Locator("text=test").First()
	if err := target.WaitFor(); err != nil {
		t.Fatalf("wait for target row: %v", err)
	}
	t.Log("target row ready")

	t.Log("starting drag")
	if err := source.DragTo(target); err != nil {
		t.Fatalf("drag hello.txt to test folder: %v", err)
	}
	t.Log("drag finished")

	rootBody := waitForDriveBody(t, bodyText)
	t.Logf("root body after drag: %q", rootBody)
	if strings.Contains(rootBody, "hello.txt") {
		t.Fatalf("expected hello.txt to leave root after move, got %q", rootBody)
	}

	openDriveDir(t, bodyText, openDir, "test")
	t.Log("opened target directory")

	testBody := waitForDriveBody(t, bodyText)
	t.Logf("target body after drag: %q", testBody)
	if !strings.Contains(testBody, "hello.txt") {
		t.Fatalf("expected hello.txt inside /test after move, got %q", testBody)
	}
}
