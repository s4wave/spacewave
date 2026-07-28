//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

const driveBrowserUIWaitTimeout = 60 * time.Second

func TestGoScriptDriveBrowserPreviewRenameUIParity(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := harness(t).NewCleanSession(t)
	page := sess.Page()
	if err := page.SetViewportSize(1440, 900); err != nil {
		t.Fatalf("set viewport size: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page = scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)
	waitForStarterDriveGuidance(t, page)

	openGettingStartedFile(t, page)
	clickDriveToolbarButton(t, page, "Up")
	waitForDriveEntry(t, page, gettingStartedFileName)

	previewFile := playwright.InputFile{
		Name:     "row3-preview.md",
		MimeType: "text/markdown",
		Buffer:   []byte("# Row 3 preview\n\nrow3 preview body survives reload.\n"),
	}
	uploadDriveFileThroughUI(t, page, previewFile)
	openDriveEntryViaContextMenu(t, page, previewFile.Name)
	waitForUnixFSFileText(t, page, "preview upload", "row3 preview body survives reload")
	clickDriveToolbarButton(t, page, "Up")
	renameDriveEntryViaContextMenu(t, page, previewFile.Name, "row3-renamed.md")
	waitForDriveEntry(t, page, "row3-renamed.md")

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after GoScript Drive preview/rename parity: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after GoScript Drive preview/rename parity: %+v", report)
	}
}

func TestGoScriptDriveBrowserMoveDragDeleteUIParity(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := harness(t).NewCleanSession(t)
	page := sess.Page()
	if err := page.SetViewportSize(1440, 900); err != nil {
		t.Fatalf("set viewport size: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page = scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)
	waitForStarterDriveGuidance(t, page)

	createDriveFolder(t, page, "row5-target")

	moveFile := playwright.InputFile{
		Name:     "row5-move-dialog.md",
		MimeType: "text/markdown",
		Buffer:   []byte("row5 move dialog file\n"),
	}
	uploadDriveFileThroughUI(t, page, moveFile)
	moveDriveEntryViaContextMenu(t, page, moveFile.Name, "row5-target")

	dragFile := playwright.InputFile{
		Name:     "row5-drag-move.md",
		MimeType: "text/markdown",
		Buffer:   []byte("row5 drag move file\n"),
	}
	uploadDriveFileThroughUI(t, page, dragFile)
	dropDriveEntryOnFolder(t, page, dragFile.Name, "row5-target")
	waitForDriveEntryGone(t, page, dragFile.Name)

	deleteFile := playwright.InputFile{
		Name:     "row5-delete.md",
		MimeType: "text/markdown",
		Buffer:   []byte("row5 delete me\n"),
	}
	uploadDriveFileThroughUI(t, page, deleteFile)
	deleteDriveEntryViaContextMenu(t, page, deleteFile.Name)
	waitForDriveEntriesPresentAndAbsent(t, page, []string{gettingStartedFileName, "row5-target"}, []string{deleteFile.Name})

	openDriveEntry(t, page, "row5-target")
	waitForDriveEntry(t, page, moveFile.Name)
	waitForDriveEntry(t, page, dragFile.Name)
	waitForDriveEntryGone(t, page, deleteFile.Name)

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after GoScript Drive move/drag/delete parity: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after GoScript Drive move/drag/delete parity: %+v", report)
	}
}

func TestGoScriptDriveBrowserLayoutDropReloadUIParity(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := harness(t).NewCleanSession(t)
	page := sess.Page()
	if err := page.SetViewportSize(1440, 900); err != nil {
		t.Fatalf("set viewport size: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page = scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)
	waitForStarterDriveGuidance(t, page)

	layoutFile := playwright.InputFile{
		Name:     "row6-layout.md",
		MimeType: "text/markdown",
		Buffer:   []byte("row6 layout drag body survives shell tab drop and reload.\n"),
	}
	uploadDriveFileThroughUI(t, page, layoutFile)
	dropUnixFSOpenableOnShellLayout(t, page, scenario, layoutFile.Name)
	waitForUnixFSFileText(t, page, "row6 layout drop", "row6 layout drag body survives shell tab drop and reload")

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload Drive layout state: %v", err)
	}
	WaitForApp(t, page)
	waitForUnixFSFileText(t, page, "row6 layout drop after reload", "row6 layout drag body survives shell tab drop and reload")
	clickDriveToolbarButton(t, page, "Up")
	waitForDriveEntry(t, page, gettingStartedFileName)
	waitForDriveEntry(t, page, layoutFile.Name)

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after GoScript Drive layout drop/reload parity: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after GoScript Drive layout drop/reload parity: %+v", report)
	}
}

func waitForStarterDriveGuidance(t testing.TB, page playwright.Page) {
	t.Helper()

	if err := page.Locator(driveWelcomeSelector).First().WaitFor(); err != nil {
		t.Fatalf("wait for Drive starter welcome guidance: %v", err)
	}
	if err := page.Locator(driveInviteCTASelector).First().WaitFor(); err != nil {
		t.Fatalf("wait for Drive invite CTA guidance: %v", err)
	}
}

func uploadDriveFileThroughUI(t testing.TB, page playwright.Page, file playwright.InputFile) {
	t.Helper()

	UploadViaPicker(t, page, []playwright.InputFile{file})
	waitForDriveUploadSummary(t, page, "1/1 uploaded")
	waitForDriveEntry(t, page, file.Name)
	clearDriveUploadDone(t, page)
}

func waitForUnixFSFileText(t testing.TB, page playwright.Page, label string, want string) {
	t.Helper()

	err := page.Locator("[data-testid='unixfs-browser'] pre").Filter(playwright.LocatorFilterOptions{
		HasText: want,
	}).First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(driveBrowserUIWaitTimeout / time.Millisecond)),
	})
	if err != nil {
		failWithPageBody(t, page, "wait for "+label+" file text", err)
	}
}

func waitForDriveEntryGone(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	_, err := page.WaitForFunction(`({ name }) => {
		return !Array.from(document.querySelectorAll('[data-testid="unixfs-browser"] [role="row"]')).some((el) => {
			return el instanceof HTMLElement && el.textContent?.includes(name)
		})
	}`, map[string]any{"name": name}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(float64(driveBrowserUIWaitTimeout / time.Millisecond)),
	})
	if err != nil {
		failWithPageBody(t, page, "wait for drive entry to disappear "+name, err)
	}
}

func waitForDriveEntriesPresentAndAbsent(t testing.TB, page playwright.Page, presentNames, absentNames []string) {
	t.Helper()

	_, err := page.WaitForFunction(`({ presentNames, absentNames }) => {
		const rows = Array.from(document.querySelectorAll('[data-testid="unixfs-browser"] [role="row"]'))
			.filter((el) => el instanceof HTMLElement)
			.map((el) => el.textContent || '')
		if (!presentNames.every((name) => rows.some((text) => text.includes(name)))) {
			return false
		}
		return absentNames.every((name) => !rows.some((text) => text.includes(name)))
	}`, map[string]any{
		"presentNames": presentNames,
		"absentNames":  absentNames,
	}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(float64(driveBrowserUIWaitTimeout / time.Millisecond)),
	})
	if err != nil {
		failWithPageBody(t, page, "wait for Drive listing present/absent entries", err)
	}
}

func clickDriveToolbarButton(t testing.TB, page playwright.Page, title string) {
	t.Helper()

	if err := page.Locator("button[title='" + title + "']:not([disabled])").First().Click(); err != nil {
		failWithPageBody(t, page, "click Drive toolbar "+title, err)
	}
	waitForDriveSettled(t, page)
}

func openDriveEntryContextMenu(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	row := visibleDriveBrowser(page).Locator("[role='row']:has-text('" + name + "')").First()
	if err := row.WaitFor(); err != nil {
		failWithPageBody(t, page, "wait for Drive entry row "+name, err)
	}

	_, err := page.Evaluate(`({ name }) => {
		const browser = document.querySelector('[data-testid="unixfs-browser"]')
		if (!(browser instanceof HTMLElement)) {
			throw new Error('drive browser not found')
		}
		const row = Array.from(browser.querySelectorAll('[role="row"]')).find((el) => {
			return el instanceof HTMLElement && el.textContent?.includes(name)
		})
		if (!(row instanceof HTMLElement)) {
			throw new Error('drive entry row not found: ' + name)
		}
		const rect = row.getBoundingClientRect()
		row.dispatchEvent(new MouseEvent('contextmenu', {
			bubbles: true,
			cancelable: true,
			view: window,
			clientX: rect.left + 16,
			clientY: rect.top + Math.min(rect.height - 4, 16),
		}))
	}`, map[string]any{"name": name})
	if err != nil {
		failWithPageBody(t, page, "open context menu for "+name, err)
	}
}

func clickDropdownMenuItem(t testing.TB, page playwright.Page, label string) {
	t.Helper()

	_, err := page.Evaluate(`({ label }) => {
		const item = Array.from(document.querySelectorAll('[role="menuitem"]')).find((el) => {
			return el instanceof HTMLElement && (el.textContent || '').includes(label)
		})
		if (!(item instanceof HTMLElement)) {
			throw new Error('menu item not found: ' + label)
		}
		item.click()
	}`, map[string]any{"label": label})
	if err != nil {
		failWithPageBody(t, page, "click menu item "+label, err)
	}
}

func renameDriveEntryViaContextMenu(t testing.TB, page playwright.Page, oldName, newName string) {
	t.Helper()

	openDriveEntryContextMenu(t, page, oldName)
	clickDropdownMenuItem(t, page, "Rename")
	input := page.Locator("[data-testid='unixfs-browser'] input").First()
	if err := input.WaitFor(); err != nil {
		t.Fatalf("wait for rename input %s: %v", oldName, err)
	}
	if err := input.Fill(newName); err != nil {
		t.Fatalf("fill rename input %s -> %s: %v", oldName, newName, err)
	}
	if err := input.Press("Enter"); err != nil {
		t.Fatalf("confirm rename %s -> %s: %v", oldName, newName, err)
	}
	waitForDriveEntry(t, page, newName)
	waitForDriveEntryGone(t, page, oldName)
}

func openDriveEntryViaContextMenu(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	openDriveEntryContextMenu(t, page, name)
	clickDropdownMenuItem(t, page, "Open")
	waitForDriveSettled(t, page)
}

func moveDriveEntryViaContextMenu(t testing.TB, page playwright.Page, name, target string) {
	t.Helper()

	openDriveEntryContextMenu(t, page, name)
	clickDropdownMenuItem(t, page, "Move")
	dialog := page.Locator("[role='dialog']:has-text('Move " + name + "')").First()
	if err := dialog.WaitFor(); err != nil {
		t.Fatalf("wait for move dialog %s: %v", name, err)
	}
	if err := page.Locator("[cmdk-item]:has-text('" + target + "')").First().Click(); err != nil {
		t.Fatalf("select move target %s for %s: %v", target, name, err)
	}
	if err := page.Locator("[role='dialog'] button:has-text('Move')").Last().Click(); err != nil {
		t.Fatalf("confirm move %s to %s: %v", name, target, err)
	}
	waitForDriveEntryGone(t, page, name)
}

func deleteDriveEntryViaContextMenu(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	openDriveEntryContextMenu(t, page, name)
	clickDropdownMenuItem(t, page, "Delete")
	dialog := page.Locator("[role='dialog']:has-text('Delete item')").First()
	if err := dialog.WaitFor(); err != nil {
		t.Fatalf("wait for delete dialog %s: %v", name, err)
	}
	if err := dialog.Locator("button:has-text('Delete')").Last().Click(); err != nil {
		t.Fatalf("confirm delete %s: %v", name, err)
	}
	waitForDriveEntryGone(t, page, name)
}

func driveEntryAppDragEnvelope(t testing.TB, page playwright.Page, name string, wantOpenable bool) string {
	t.Helper()

	raw, err := page.Evaluate(`({ name, wantOpenable }) => {
		const mime = 'application/x-s4wave-app-drag+json'
		const row = Array.from(document.querySelectorAll('[data-testid="unixfs-browser"] [role="row"]')).find((el) => {
			return el instanceof HTMLElement && el.textContent?.includes(name)
		})
		if (!(row instanceof HTMLElement)) {
			throw new Error('drive entry row not found: ' + name)
		}
		const transfer = new DataTransfer()
		row.dispatchEvent(new DragEvent('dragstart', {
			bubbles: true,
			cancelable: true,
			dataTransfer: transfer,
		}))
		const raw = transfer.getData(mime)
		if (!raw) {
			throw new Error('drive row dragstart did not write app-drag payload: ' + name)
		}
		const envelope = JSON.parse(raw)
		if (envelope.version !== 1 || !Array.isArray(envelope.items) || envelope.items.length !== 1) {
			throw new Error('unexpected app-drag envelope shape: ' + raw)
		}
		const item = envelope.items[0]
		if (item.label !== name || !Array.isArray(item.capabilities)) {
			throw new Error('unexpected app-drag item for ' + name + ': ' + raw)
		}
		const movable = item.capabilities.find((cap) => {
			return cap.kind === 'movable' &&
				cap.value?.case === 'unixfs-entry' &&
				cap.value?.value?.unixfsId === 'files' &&
				cap.value?.value?.path === '/' + name &&
				cap.value?.value?.isDir === false
		})
		if (!movable) {
			throw new Error('drive row app-drag payload missing movable unixfs-entry for ' + name + ': ' + raw)
		}
		const openable = item.capabilities.find((cap) => {
			return cap.kind === 'openable' &&
				cap.value?.case === 'object' &&
				cap.value?.value?.objectInfo?.info?.case === 'unixfsObjectInfo' &&
				cap.value?.value?.objectInfo?.info?.value?.unixfsId === 'files' &&
				cap.value?.value?.objectInfo?.info?.value?.path === '/' + name &&
				typeof cap.value?.value?.routePath === 'string'
		})
		if (wantOpenable && !openable) {
			throw new Error('drive row app-drag payload missing openable object for ' + name + ': ' + raw)
		}
		row.dispatchEvent(new DragEvent('dragend', {
			bubbles: true,
			cancelable: true,
			dataTransfer: transfer,
		}))
		return raw
	}`, map[string]any{
		"name":         name,
		"wantOpenable": wantOpenable,
	})
	if err != nil {
		t.Fatalf("capture app-drag payload for %s: %v", name, err)
	}
	payload, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected app-drag payload for %s: %#v", name, raw)
	}
	return payload
}

func dropDriveEntryOnFolder(t testing.TB, page playwright.Page, entryName string, targetName string) {
	t.Helper()

	payload := driveEntryAppDragEnvelope(t, page, entryName, false)
	_, err := page.Evaluate(`({ entryName, targetName, payload }) => {
		const target = Array.from(document.querySelectorAll('[data-testid="unixfs-browser"] [role="row"]'))
			.find((el) => el instanceof HTMLElement && el.textContent?.includes(targetName))
		if (!(target instanceof HTMLElement)) {
			throw new Error('target row not found: ' + targetName)
		}
		const transfer = new DataTransfer()
		transfer.setData('application/x-s4wave-app-drag+json', payload)
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
		"payload":    payload,
	})
	if err != nil {
		t.Fatalf("drop %s on folder %s: %v", entryName, targetName, err)
	}
}

func dropUnixFSOpenableOnShellLayout(t testing.TB, page playwright.Page, scenario *DriveScenario, name string) {
	t.Helper()

	payload := driveEntryAppDragEnvelope(t, page, name, true)
	routePath := fmt.Sprintf(
		"/u/%d/so/%s/-/files/-%s",
		scenario.GetSessionIndex(),
		scenario.GetSpaceID(),
		driveBrowserUIEscapedRouteSuffix(name),
	)
	_, err := page.Evaluate(`({ name, payload }) => {
		const row = Array.from(document.querySelectorAll('[data-testid="unixfs-browser"] [role="row"]')).find((el) => {
			return el instanceof HTMLElement && el.textContent?.includes(name)
		})
		if (!(row instanceof HTMLElement)) {
			throw new Error('drive entry row not found: ' + name)
		}
		const target =
			document.querySelector('.shell-flexlayout .flexlayout__tabset') ||
			document.querySelector('.shell-flexlayout .flexlayout__layout') ||
			document.querySelector('.shell-flexlayout')
		if (!(target instanceof HTMLElement)) {
			throw new Error('shell layout drop target not found')
		}
		const transfer = new DataTransfer()
		transfer.setData('application/x-s4wave-app-drag+json', payload)
		const rect = target.getBoundingClientRect()
		const eventInit = {
			bubbles: true,
			cancelable: true,
			dataTransfer: transfer,
			clientX: rect.left + Math.max(24, rect.width - 80),
			clientY: rect.top + Math.max(24, rect.height / 2),
		}
		row.dispatchEvent(new DragEvent('dragstart', eventInit))
		for (const type of ['dragenter', 'dragover', 'drop']) {
			target.dispatchEvent(new DragEvent(type, eventInit))
		}
		row.dispatchEvent(new DragEvent('dragend', eventInit))
	}`, map[string]any{
		"name":    name,
		"payload": payload,
	})
	if err != nil {
		t.Fatalf("drop %s on shell layout: %v", name, err)
	}

	_, err = page.WaitForFunction(`({ routePath }) => {
		const state = sessionStorage.getItem('shell-tabs-state') || ''
		return state.includes(routePath) || window.location.hash.includes('/g/')
	}`, map[string]any{"routePath": routePath}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(float64(driveBrowserUIWaitTimeout / time.Millisecond)),
	})
	if err != nil {
		failWithPageBody(t, page, "wait for shell layout tab drop "+name, err)
	}
}

func driveBrowserUIEscapedRouteSuffix(name string) string {
	escaped := url.PathEscape(name)
	if strings.HasPrefix(escaped, "/") {
		return escaped
	}
	return "/" + escaped
}
