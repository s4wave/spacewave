//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

func dropUnixFSEntryOnFolder(t testing.TB, page playwright.Page, entryName string, targetName string) {
	t.Helper()

	_, err := page.Evaluate(`({ entryName, targetName }) => {
		const target = Array.from(document.querySelectorAll('[role="row"]'))
			.find((el) => el instanceof HTMLElement && el.textContent?.includes(targetName))
		if (!(target instanceof HTMLElement)) {
			throw new Error(`+"`"+`target row not found: ${targetName}`+"`"+`)
		}
		const transfer = new DataTransfer()
		transfer.setData('application/x-s4wave-app-drag+json', JSON.stringify({
			version: 1,
			items: [{
				id: entryName,
				label: entryName,
				capabilities: [{
					kind: 'movable',
					value: {
						case: 'unixfs-entry',
						value: {
							unixfsId: 'files',
							path: '/' + entryName,
							isDir: false,
						},
					},
				}],
			}],
		}))
		for (const type of ['dragover', 'drop']) {
			target.dispatchEvent(new DragEvent(type, {
				bubbles: true,
				cancelable: true,
				dataTransfer: transfer,
			}))
		}
	}`, map[string]any{
		"entryName":  entryName,
		"targetName": targetName,
	})
	if err != nil {
		t.Fatalf("drop %s on folder %s: %v", entryName, targetName, err)
	}
}

func waitForDriveBody(t testing.TB, pageText func() (string, error)) string {
	t.Helper()

	body, err := pageText()
	if err != nil {
		t.Fatalf("read drive browser text: %v", err)
	}
	return strings.TrimSpace(body)
}

func openDriveDir(t testing.TB, open func(name string), name string) {
	t.Helper()

	open(name)
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
	UploadViaPicker(t, page, []playwright.InputFile{
		{
			Name:     "hello.txt",
			MimeType: "text/plain",
			Buffer:   []byte("hello from move test"),
		},
	})
	waitForDriveEntry(t, page, "hello.txt")
	createDriveFolder(t, page, "test")
	t.Log("drive ready")

	bodyText := func() (string, error) {
		return browser.TextContent()
	}
	openDir := func(name string) {
		t.Helper()

		row := page.Locator("[role='row']:has-text('" + name + "')").First()
		if err := row.WaitFor(); err != nil {
			t.Fatalf("wait for %s row: %v", name, err)
		}
		if err := row.Dblclick(); err != nil {
			t.Fatalf("open %s row: %v", name, err)
		}
	}

	source := page.Locator("[role='row']:has-text('hello.txt')").First()
	if err := source.WaitFor(); err != nil {
		t.Fatalf("wait for source row: %v", err)
	}
	t.Log("source row ready")

	target := page.Locator("[role='row']:has-text('test')").First()
	if err := target.WaitFor(); err != nil {
		t.Fatalf("wait for target row: %v", err)
	}
	t.Log("target row ready")

	t.Log("starting folder drop")
	dropUnixFSEntryOnFolder(t, page, "hello.txt", "test")
	t.Log("folder drop finished")

	if err := source.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateDetached,
	}); err != nil {
		t.Fatalf("wait for source row to leave root: %v", err)
	}
	rootBody := waitForDriveBody(t, bodyText)
	t.Logf("root body after drag: %q", rootBody)
	if strings.Contains(rootBody, "hello.txt") {
		t.Fatalf("expected hello.txt to leave root after move, got %q", rootBody)
	}

	openDriveDir(t, openDir, "test")
	t.Log("opened target directory")

	waitForDriveEntry(t, page, "hello.txt")
	testBody := waitForDriveBody(t, bodyText)
	t.Logf("target body after drag: %q", testBody)
	if !strings.Contains(testBody, "hello.txt") {
		t.Fatalf("expected hello.txt inside /test after move, got %q", testBody)
	}
}
