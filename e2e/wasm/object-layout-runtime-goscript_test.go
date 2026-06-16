//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
)

func TestGoScriptObjectLayoutRuntimeParity(t *testing.T) {
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

	prepare := runObjectLayoutRuntimeScript(t, page, "prepare")
	objectHash := stringField(prepare, "objectHash")
	if objectHash == "" {
		t.Fatalf("prepare did not return object route: %#v", prepare)
	}
	assertObjectLayoutTabs(t, prepare, []layoutTabExpectation{
		{id: "files", name: "Files"},
	})

	NavigateHash(t, harness(t), page, objectHash)
	waitForObjectLayoutRoute(t, page)
	waitForDriveEntry(t, page, gettingStartedFileName)

	layoutFile := playwright.InputFile{
		Name:     "row7-object-layout.md",
		MimeType: "text/markdown",
		Buffer:   []byte("row7 object layout drag body survives route reload.\n"),
	}
	uploadDriveFileThroughUI(t, page, layoutFile)
	dropUnixFSOpenableOnObjectLayout(t, page, layoutFile.Name)
	waitForUnixFSFileText(t, page, "object layout drag tab", "row7 object layout drag body survives route reload")

	waitForObjectLayoutTabs(t, page, []layoutTabExpectation{
		{id: "files"},
		{name: layoutFile.Name, infoCase: "unixfsObjectInfo", unixfsPath: "/" + layoutFile.Name},
	})

	mutated := runObjectLayoutRuntimeScript(t, page, "typed-mutate")
	if got := stringField(mutated, "navigatedPath"); got != "/getting-started.md" {
		t.Fatalf("typed NavigateTab path=%q want /getting-started.md: %#v", got, mutated)
	}
	if got := stringField(mutated, "replacedName"); got != "Files Replaced" {
		t.Fatalf("typed ReplaceTab name=%q want Files Replaced: %#v", got, mutated)
	}
	assertObjectLayoutTabs(t, mutated, []layoutTabExpectation{
		{id: "files", name: "Files Replaced"},
		{name: layoutFile.Name, infoCase: "unixfsObjectInfo", unixfsPath: "/" + layoutFile.Name},
	})

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload ObjectLayout route: %v", err)
	}
	WaitForApp(t, page)
	waitForObjectLayoutRoute(t, page)
	afterReload := runObjectLayoutRuntimeScript(t, page, "inspect")
	assertObjectLayoutTabs(t, afterReload, []layoutTabExpectation{
		{id: "files", name: "Files Replaced"},
		{name: layoutFile.Name, infoCase: "unixfsObjectInfo", unixfsPath: "/" + layoutFile.Name},
	})

	NavigateHash(t, harness(t), page, "#/")
	NavigateHash(t, harness(t), page, objectHash)
	waitForObjectLayoutRoute(t, page)
	afterReopen := runObjectLayoutRuntimeScript(t, page, "inspect")
	assertObjectLayoutTabs(t, afterReopen, []layoutTabExpectation{
		{id: "files", name: "Files Replaced"},
		{name: layoutFile.Name, infoCase: "unixfsObjectInfo", unixfsPath: "/" + layoutFile.Name},
	})

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after GoScript ObjectLayout parity: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after GoScript ObjectLayout parity: %+v", report)
	}
}

func TestGoScriptObjectLayoutSeedModelParity(t *testing.T) {
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

	seeded := runObjectLayoutRuntimeScript(t, page, "seed-model")
	objectHash := stringField(seeded, "objectHash")
	if objectHash == "" {
		t.Fatalf("seed-model did not return object route: %#v", seeded)
	}
	assertObjectLayoutTabsFromField(t, seeded, "sameMountTabs", []layoutTabExpectation{
		{id: "files", name: "Files", infoCase: "worldObjectInfo"},
	})
	assertObjectLayoutTabs(t, seeded, []layoutTabExpectation{
		{id: "files", name: "Files", infoCase: "worldObjectInfo"},
	})

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after GoScript ObjectLayout seed model proof: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after GoScript ObjectLayout seed model proof: %+v", report)
	}
}

type layoutTabExpectation struct {
	id         string
	name       string
	infoCase   string
	unixfsPath string
}

func runObjectLayoutRuntimeScript(t testing.TB, page playwright.Page, action string) map[string]any {
	t.Helper()

	raw, err := page.Evaluate(harness(t).Script("object-layout-runtime-parity.ts"), map[string]any{
		"action":     action,
		"deadlineMs": 120000,
	})
	if err != nil {
		t.Fatalf("run ObjectLayout runtime script %s: %v", action, err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected ObjectLayout runtime result %T: %#v", raw, raw)
	}
	return result
}

func waitForObjectLayoutRoute(t testing.TB, page playwright.Page) {
	t.Helper()

	wait := playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64((120 * time.Second) / time.Millisecond)),
	}
	if err := page.Locator(".space-flexlayout").First().WaitFor(wait); err != nil {
		failWithPageBody(t, page, "wait for ObjectLayout shell", err)
	}
	if err := page.Locator(".space-flexlayout [data-testid='unixfs-browser']").First().WaitFor(wait); err != nil {
		failWithPageBody(t, page, "wait for ObjectLayout files tab", err)
	}
}

func dropUnixFSOpenableOnObjectLayout(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	payload := driveEntryAppDragEnvelope(t, page, name, true)
	_, err := page.Evaluate(`({ name, payload }) => {
		const row = Array.from(document.querySelectorAll('.space-flexlayout [data-testid="unixfs-browser"] [role="row"]')).find((el) => {
			return el instanceof HTMLElement && el.textContent?.includes(name)
		})
		if (!(row instanceof HTMLElement)) {
			throw new Error('ObjectLayout drive entry row not found: ' + name)
		}
		const target =
			document.querySelector('.space-flexlayout .flexlayout__tabset') ||
			document.querySelector('.space-flexlayout .flexlayout__layout') ||
			document.querySelector('.space-flexlayout')
		if (!(target instanceof HTMLElement)) {
			throw new Error('ObjectLayout drop target not found')
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
		t.Fatalf("drop %s on ObjectLayout: %v", name, err)
	}
}

func assertObjectLayoutTabs(t testing.TB, result map[string]any, want []layoutTabExpectation) {
	t.Helper()

	assertObjectLayoutTabsFromField(t, result, "tabs", want)
}

func waitForObjectLayoutTabs(t testing.TB, page playwright.Page, want []layoutTabExpectation) map[string]any {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	var last map[string]any
	for {
		result := runObjectLayoutRuntimeScript(t, page, "inspect")
		tabs, ok := result["tabs"].([]any)
		if ok {
			matched := true
			for _, expectation := range want {
				if !objectLayoutTabsContain(tabs, expectation) {
					matched = false
					break
				}
			}
			if matched {
				return result
			}
		}
		last = result
		if !time.Now().Before(deadline) {
			break
		}
		<-tick.C
	}
	assertObjectLayoutTabs(t, last, want)
	return last
}

func assertObjectLayoutTabsFromField(t testing.TB, result map[string]any, field string, want []layoutTabExpectation) {
	t.Helper()

	tabs, ok := result[field].([]any)
	if !ok {
		t.Fatalf("ObjectLayout result missing %s: %#v", field, result)
	}
	for _, expectation := range want {
		if !objectLayoutTabsContain(tabs, expectation) {
			t.Fatalf("ObjectLayout %s missing %+v in %s: %#v", field, expectation, formatObjectLayoutTabs(tabs), result)
		}
	}
}

func objectLayoutTabsContain(tabs []any, want layoutTabExpectation) bool {
	for _, raw := range tabs {
		tab, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if want.id != "" && stringField(tab, "id") != want.id {
			continue
		}
		if want.name != "" && stringField(tab, "name") != want.name {
			continue
		}
		if want.infoCase != "" && stringField(tab, "infoCase") != want.infoCase {
			continue
		}
		if want.unixfsPath != "" && stringField(tab, "unixfsPath") != want.unixfsPath {
			continue
		}
		return true
	}
	return false
}

func formatObjectLayoutTabs(tabs []any) string {
	parts := make([]string, 0, len(tabs))
	for _, raw := range tabs {
		tab, ok := raw.(map[string]any)
		if !ok {
			parts = append(parts, fmt.Sprintf("%#v", raw))
			continue
		}
		parts = append(parts, fmt.Sprintf(
			"{id:%q name:%q info:%q unixfs:%q}",
			stringField(tab, "id"),
			stringField(tab, "name"),
			stringField(tab, "infoCase"),
			stringField(tab, "unixfsPath"),
		))
	}
	return strings.Join(parts, ", ")
}
