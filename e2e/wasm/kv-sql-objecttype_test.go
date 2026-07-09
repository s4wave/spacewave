//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

const objectTypeQuickstartWaitMS = 600000

type objectTypeQuickstartScenario struct {
	sessionIndex uint32
	spaceID      string
}

func TestQuickstartKvEditPersistsAfterReload(t *testing.T) {
	sess := harness(t).NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer assertNoObjectTypeBrowserCrash(t, console, "KV quickstart")

	page := sess.Page()
	scenario := createObjectTypeQuickstartScenario(t, harness(t), page, "kv", "kv/store", []string{
		"Key/Value Store",
		"hello",
	})

	selectKvKey(t, page, "hello")
	beforeSeqno := readObjectTypeSeqno(t, harness(t), page)
	const editedValue = "browser quickstart persisted value"
	valueEditor := page.Locator("textarea[aria-label='Key value']").First()
	if err := valueEditor.Fill(editedValue, playwright.LocatorFillOptions{
		Timeout: playwright.Float(objectTypeQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("fill KV value: %v\ndebug: %v", err, collectObjectTypeQuickstartDebug(page))
	}
	clickObjectTypeButton(t, page, "Save")

	kvAfterSave := readKvQuickstartValue(t, harness(t), page, "hello", beforeSeqno)
	assertKvValue(t, kvAfterSave, "hello", editedValue)

	reloadObjectTypeRoute(t, page, func() {
		waitForObjectTypeRoute(t, page, "kv/store", []string{
			"Key/Value Store",
			"hello",
		})
	})
	kvAfterReload := readKvQuickstartValue(t, harness(t), page, "hello", "")
	assertKvValue(t, kvAfterReload, "hello", editedValue)

	NavigateHash(t, harness(t), page, scenario.objectHash("kv/store"))
	waitForObjectTypeRoute(t, page, "kv/store", []string{
		"Key/Value Store",
		"hello",
	})
}

func TestQuickstartSqlRunCreatesLinkedQueryResult(t *testing.T) {
	sess := harness(t).NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer assertNoObjectTypeBrowserCrash(t, console, "SQL query quickstart")

	page := sess.Page()
	scenario := createObjectTypeQuickstartScenario(t, harness(t), page, "sql", "sql/db", []string{
		"SQL Database",
		"quickstart",
	})

	NavigateHash(t, harness(t), page, scenario.objectHash("sql/query/example"))
	waitForObjectTypeRoute(t, page, "sql/query/example", []string{
		"SQL Query",
		"SELECT name, role FROM quickstart.people WHERE id = ?",
	})
	// The target database key renders inside an editable input, whose value is
	// not part of document.body.textContent, so it cannot be a route-wait body
	// text needle. Assert the wiring at the input value instead.
	assertTargetDbInputValue(t, page, "sql/db")
	clickObjectTypeButton(t, page, "Run")
	waitForSqlQueryResult(t, page)

	linkage := readSqlQueryResultLinkage(t, harness(t), page)
	resultKeys := stringSliceField(t, linkage, "resultObjectKeys")
	if len(resultKeys) != 1 {
		t.Fatalf("sql/query-result object count = %d, want 1: %#v", len(resultKeys), linkage)
	}
	if got := stringField(linkage, "sourceQueryObjectKey"); got != "sql/query/example" {
		t.Fatalf("result source query = %q, want sql/query/example: %#v", got, linkage)
	}
	if got := stringField(linkage, "targetDbObjectKey"); got != "sql/db" {
		t.Fatalf("result target db = %q, want sql/db: %#v", got, linkage)
	}
	if got := stringField(linkage, "rowCount"); got != "1" {
		t.Fatalf("result row count = %q, want 1: %#v", got, linkage)
	}
	if got := intField(linkage, "producedByQuadCount"); got != 1 {
		t.Fatalf("produced-by quad count = %d, want 1: %#v", got, linkage)
	}
	if got := intField(linkage, "againstQuadCount"); got != 1 {
		t.Fatalf("against quad count = %d, want 1: %#v", got, linkage)
	}
}

func TestQuickstartSqlWorkbenchPinsPersistAfterReload(t *testing.T) {
	sess := harness(t).NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer assertNoObjectTypeBrowserCrash(t, console, "SQL workbench quickstart")

	page := sess.Page()
	scenario := createObjectTypeQuickstartScenario(t, harness(t), page, "sql", "sql/db", []string{
		"SQL Database",
		"quickstart",
	})

	setup := prepareSqlWorkbenchPins(t, harness(t), page)
	assertSqlWorkbenchPins(t, setup)
	workbenchKey := stringField(setup, "workbenchObjectKey")
	NavigateHash(t, harness(t), page, scenario.objectHash(workbenchKey))
	waitForObjectTypeRoute(t, page, workbenchKey, []string{
		"SQL Workbench",
		"sql/db",
		"Pinned Queries",
		"example",
		"e2e-second",
	})

	reloadObjectTypeRoute(t, page, func() {
		waitForObjectTypeRoute(t, page, workbenchKey, []string{
			"SQL Workbench",
			"sql/db",
			"Pinned Queries",
			"example",
			"e2e-second",
		})
	})
	afterReload := readSqlWorkbenchPins(t, harness(t), page)
	assertSqlWorkbenchPins(t, afterReload)
}

func createObjectTypeQuickstartScenario(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	quickstartID string,
	objectKey string,
	texts []string,
) *objectTypeQuickstartScenario {
	t.Helper()

	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/"+quickstartID)
	waitForObjectTypeRoute(t, page, objectKey, texts)

	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse %s quickstart route: %v", quickstartID, err)
	}
	return &objectTypeQuickstartScenario{
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

func (s *objectTypeQuickstartScenario) objectHash(objectKey string) string {
	return fmt.Sprintf("#/u/%d/so/%s/-/%s", s.sessionIndex, s.spaceID, objectKey)
}

func waitForObjectTypeRoute(t testing.TB, page playwright.Page, objectKey string, texts []string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const { objectKey, texts } = Array.isArray(arg) ? arg[0] : arg
		const timing =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		if (timing?.state === 'error') {
			throw new Error('quickstart failed: ' + (timing.error ?? 'unknown error'))
		}
		const hash = window.location.hash
		if (!hash.includes('/u/') || !hash.includes('/so/') || !hash.endsWith('/' + objectKey)) {
			return false
		}
		const text = document.querySelector('#bldr-root')?.textContent ?? document.body.textContent ?? ''
		if (
			text.includes('unavailable') ||
			text.includes('Run failed') ||
			text.includes('Could not open query editor')
		) {
			throw new Error(text.replace(/\s+/g, ' ').slice(0, 1200))
		}
		return texts.every((needle) => text.includes(needle))
	}`, []any{map[string]any{
		"objectKey": objectKey,
		"texts":     texts,
	}}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(objectTypeQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for object route %q: %v\ndebug: %v", objectKey, err, collectObjectTypeQuickstartDebug(page))
	}
}

func assertTargetDbInputValue(t testing.TB, page playwright.Page, want string) {
	t.Helper()

	input := page.Locator("input[aria-label='Target database object key']").First()
	got, err := input.InputValue(playwright.LocatorInputValueOptions{
		Timeout: playwright.Float(objectTypeQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("read target database input: %v\ndebug: %v", err, collectObjectTypeQuickstartDebug(page))
	}
	if got != want {
		t.Fatalf("target database input value = %q, want %q\ndebug: %v", got, want, collectObjectTypeQuickstartDebug(page))
	}
}

func waitForSqlQueryResult(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.WaitForFunction(`() => {
		const hash = window.location.hash
		if (!hash.includes('/sql/query/example/results/')) {
			return false
		}
		const text = document.querySelector('#bldr-root')?.textContent ?? document.body.textContent ?? ''
		if (text.includes('Query execution failed') || text.includes('Run failed')) {
			throw new Error(text.replace(/\s+/g, ' ').slice(0, 1200))
		}
		return text.includes('Query Result') &&
			text.includes('1 rows') &&
			text.includes('ada')
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(objectTypeQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for SQL query result: %v\ndebug: %v", err, collectObjectTypeQuickstartDebug(page))
	}
}

func selectKvKey(t testing.TB, page playwright.Page, key string) {
	t.Helper()

	selector := fmt.Sprintf("[role='option']:has-text('%s')", key)
	if err := page.Locator(selector).First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(objectTypeQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("select KV key %q: %v\ndebug: %v", key, err, collectObjectTypeQuickstartDebug(page))
	}
}

func clickObjectTypeButton(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	if err := page.Locator("button:has-text('" + text + "')").First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(objectTypeQuickstartWaitMS)},
	); err != nil {
		t.Fatalf("click button %q: %v\ndebug: %v", text, err, collectObjectTypeQuickstartDebug(page))
	}
}

func reloadObjectTypeRoute(t testing.TB, page playwright.Page, assertReady func()) {
	t.Helper()

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload object route: %v", err)
	}
	WaitForApp(t, page)
	assertReady()
}

func runObjectTypeQuickstartScript(t testing.TB, h *Harness, page playwright.Page, args map[string]any) map[string]any {
	t.Helper()

	raw, err := page.Evaluate(h.Script("kv-sql-objecttype.ts"), args)
	if err != nil {
		t.Fatalf("run object type quickstart helper %#v: %v\ndebug: %v", args, err, collectObjectTypeQuickstartDebug(page))
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected object type quickstart helper result %T: %#v", raw, raw)
	}
	return result
}

func readObjectTypeSeqno(t testing.TB, h *Harness, page playwright.Page) string {
	t.Helper()

	result := runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "seqno",
	})
	seqno := stringField(result, "seqno")
	if seqno == "" {
		t.Fatalf("helper returned empty seqno: %#v", result)
	}
	return seqno
}

func readKvQuickstartValue(t testing.TB, h *Harness, page playwright.Page, key, afterSeqno string) map[string]any {
	t.Helper()

	args := map[string]any{
		"action": "kv-value",
		"key":    key,
	}
	if afterSeqno != "" {
		args["afterSeqno"] = afterSeqno
	}
	return runObjectTypeQuickstartScript(t, h, page, args)
}

func assertKvValue(t testing.TB, result map[string]any, key, want string) {
	t.Helper()

	if !boolField(result, "found") {
		t.Fatalf("KV key %q was not found: %#v", key, result)
	}
	if got := stringField(result, "value"); got != want {
		t.Fatalf("KV key %q value = %q, want %q: %#v", key, got, want, result)
	}
}

func readSqlQueryResultLinkage(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	return runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "sql-linkage",
	})
}

func prepareSqlWorkbenchPins(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	return runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "prepare-workbench-pins",
	})
}

func readSqlWorkbenchPins(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	return runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "workbench-pins",
	})
}

func assertSqlWorkbenchPins(t testing.TB, result map[string]any) {
	t.Helper()

	if got := stringField(result, "targetDbObjectKey"); got != "sql/db" {
		t.Fatalf("workbench target db = %q, want sql/db: %#v", got, result)
	}
	assertStringSet(t, stringSliceField(t, result, "pinnedQueryObjectKeys"), []string{
		"sql/query/example",
		"sql/query/e2e-second",
	})
}

func stringSliceField(t testing.TB, m map[string]any, key string) []string {
	t.Helper()

	raw, ok := m[key].([]any)
	if !ok {
		t.Fatalf("expected string slice field %q in %#v", key, m)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("expected string item in field %q, got %T: %#v", key, item, m)
		}
		out = append(out, value)
	}
	return out
}

func assertStringSet(t testing.TB, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("string set length = %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			t.Fatalf("missing string %q: got=%v want=%v", value, got, want)
		}
	}
}

func assertNoObjectTypeBrowserCrash(t testing.TB, console <-chan string, label string) {
	t.Helper()

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Errorf("unexpected browser/WASM crash report during %s: %+v", label, report)
	}
	if report.HasExitedGoLoop() {
		t.Errorf("unexpected exited-Go loop during %s: %+v", label, report)
	}
}

func collectObjectTypeQuickstartDebug(page playwright.Page) any {
	debug, err := page.Evaluate(`() => {
		const startupMarks = globalThis.__swStartupMarks ?? []
		return JSON.stringify({
			url: window.location.href,
			hash: window.location.hash,
			timing: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
			startup: globalThis.__swBootStatus ?? null,
			appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 2500) ?? '',
			inputs: Array.from(document.querySelectorAll('input,textarea')).map((input) => ({
				ariaLabel: input.getAttribute('aria-label'),
				placeholder: input.getAttribute('placeholder'),
				value: input.value?.slice?.(0, 240) ?? '',
				disabled: input.disabled,
			})),
			buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
				text: button.textContent?.replace(/\s+/g, ' ').trim().slice(0, 160) ?? '',
				ariaLabel: button.getAttribute('aria-label') ?? '',
				title: button.getAttribute('title') ?? '',
				disabled: button.disabled,
			})),
			headings: Array.from(document.querySelectorAll('h1,h2,h3,[data-slot="loading-title"],[data-slot="loading-detail"]')).map((el) => ({
				tag: el.tagName,
				text: el.textContent?.replace(/\s+/g, ' ').trim().slice(0, 180) ?? '',
			})),
			bodyText: document.body.textContent?.replace(/\s+/g, ' ').slice(0, 3000) ?? '',
			startupMarks: startupMarks.slice(Math.max(0, startupMarks.length - 20)),
		})
	}`)
	if err != nil {
		return "failed to collect object type quickstart debug: " + err.Error()
	}
	if s, ok := debug.(string); ok {
		return strings.TrimSpace(s)
	}
	return debug
}
