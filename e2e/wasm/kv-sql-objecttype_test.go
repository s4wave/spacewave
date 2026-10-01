//go:build !skip_e2e && !js

package wasm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

const objectTypeQuickstartWaitMS = 600000

type objectTypeQuickstartScenario struct {
	sessionIndex uint32
	spaceID      string
}

func TestQuickstartKvEditPersistsAfterReload(t *testing.T) {
	// Start a clean page session and watch for browser crashes.
	sess := harness(t).NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer assertNoObjectTypeBrowserCrash(t, console, "KV quickstart")

	// Open the KV quickstart and parse its route.
	page := sess.Page()
	scenario := createObjectTypeQuickstartScenario(t, harness(t), page, "kv", "kv/store", []string{
		"Key/Value Store",
		"hello",
	})

	// Select the hello key and record its sequence number before editing.
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

	// Save the edited value through the KV editor.
	kvAfterSave := readKvQuickstartValue(t, harness(t), page, "hello", beforeSeqno)
	assertKvValue(t, kvAfterSave, "hello", editedValue)

	// Reload the route and assert the edited value persists.
	// Read the saved value and assert it matches the edit.
	reloadObjectTypeRoute(t, page, func() {
		waitForObjectTypeRoute(t, page, "kv/store", []string{
			"Key/Value Store",
			"hello",
		})
	})
	kvAfterReload := readKvQuickstartValue(t, harness(t), page, "hello", "")
	assertKvValue(t, kvAfterReload, "hello", editedValue)

	// Navigate back to the KV object route and confirm it renders.
	// Reload the route and assert the edited value persists.
	NavigateHash(t, harness(t), page, scenario.objectHash("kv/store"))
	waitForObjectTypeRoute(t, page, "kv/store", []string{
		"Key/Value Store",
		"hello",
	})
}

func TestQuickstartSqlRunCreatesLinkedQueryResult(t *testing.T) {
	// Start a clean page session and watch for browser crashes.
	sess := harness(t).NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer assertNoObjectTypeBrowserCrash(t, console, "SQL query quickstart")

	// Open the SQL quickstart and parse its route.
	page := sess.Page()
	scenario := createObjectTypeQuickstartScenario(t, harness(t), page, "sql", "sql/db", []string{
		"SQL Database",
		"quickstart",
	})

	// Open the example SQL query route and run it.
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

	// Read the query-result linkage and assert every field.
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
	// Start a clean page session and watch for browser crashes.
	sess := harness(t).NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer assertNoObjectTypeBrowserCrash(t, console, "SQL workbench quickstart")

	// Open the SQL quickstart and parse its route.
	page := sess.Page()
	scenario := createObjectTypeQuickstartScenario(t, harness(t), page, "sql", "sql/db", []string{
		"SQL Database",
		"quickstart",
	})

	// Prepare workbench pins and assert them on a fresh mount.
	setup := prepareSqlWorkbenchPins(t, harness(t), page)
	assertSqlWorkbenchPins(t, setup)
	afterFreshMount := readSqlWorkbenchPins(t, harness(t), page)
	assertSqlWorkbenchPins(t, afterFreshMount)
	workbenchKey := stringField(setup, "workbenchObjectKey")
	NavigateHash(t, harness(t), page, scenario.objectHash(workbenchKey))
	waitForObjectTypeRoute(t, page, workbenchKey, []string{
		"SQL Workbench",
		"sql/db",
		"Pinned Queries",
		"example",
		"e2e-second",
	})

	// Reload the workbench route and assert the pins persist.
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

// Open the object type quickstart route and wait for it to render.
func createObjectTypeQuickstartScenario(
	t testing.TB,
	h *Harness,
	page playwright.Page,
	quickstartID string,
	objectKey string,
	texts []string,
) *objectTypeQuickstartScenario {
	// Parse the quickstart route into a scenario.
	t.Helper()

	// Run the quickstart plugin preflight for SQL.
	preflightObjectTypeQuickstartPlugin(t, h, quickstartID)

	// Load the quickstart page and wait for the object route.
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/"+quickstartID)
	waitForObjectTypeRoute(t, page, objectKey, texts)

	// Parse the quickstart route into a scenario.
	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse %s quickstart route: %v", quickstartID, err)
	}
	return &objectTypeQuickstartScenario{
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

// Format the scenario route hash for the given object key.
func (s *objectTypeQuickstartScenario) objectHash(objectKey string) string {
	return fmt.Sprintf("#/u/%d/so/%s/-/%s", s.sessionIndex, s.spaceID, objectKey)
}

// Settle the SQL plugin manifest before SQL quickstarts.
func preflightObjectTypeQuickstartPlugin(t testing.TB, h *Harness, quickstartID string) {
	t.Helper()
	if quickstartID != "sql" {
		return
	}
	if err := h.SettleProjectManifest("spacewave-sql"); err != nil {
		t.Fatalf("settle SQL quickstart manifest: %v", err)
	}
}

// Poll until the object route renders with all expected text needles.
func waitForObjectTypeRoute(t testing.TB, page playwright.Page, objectKey string, texts []string) {
	t.Helper()

	// Wait for the route function to succeed or fail within the deadline.
	deadline := time.Now().Add(time.Duration(objectTypeQuickstartWaitMS) * time.Millisecond)
	for time.Now().Before(deadline) {
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
			const transientUnknownObjectType = text.includes('unknown object type')
			if (
				(text.includes('unavailable') && !transientUnknownObjectType) ||
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
			Timeout: playwright.Float(15000),
		})
		if err == nil {
			return
		}
		if !strings.Contains(err.Error(), "Timeout") {
			t.Fatalf("wait for object route %q: %v\ndebug: %v", objectKey, err, collectObjectTypeQuickstartDebug(page))
		}
		retry := page.Locator("button:has-text('Retry')").First()
		_ = retry.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(1000)})
	}
	t.Fatalf("wait for object route %q: timed out after %dms\ndebug: %v", objectKey, objectTypeQuickstartWaitMS, collectObjectTypeQuickstartDebug(page))
}

func assertTargetDbInputValue(t testing.TB, page playwright.Page, want string) {
	// Read the target database input value and compare it.
	t.Helper()

	// Read the input value within the quickstart deadline.
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

// Wait for the SQL query result page to render.
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

// Click the KV key option in the object type viewer.
func selectKvKey(t testing.TB, page playwright.Page, key string) {
	t.Helper()

	selector := fmt.Sprintf("[role='option']:has-text('%s')", key)
	if err := page.Locator(selector).First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(objectTypeQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("select KV key %q: %v\ndebug: %v", key, err, collectObjectTypeQuickstartDebug(page))
	}
}

// Click the named button in the object type viewer.
func clickObjectTypeButton(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	if err := page.Locator("button:has-text('" + text + "')").First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(objectTypeQuickstartWaitMS)},
	); err != nil {
		t.Fatalf("click button %q: %v\ndebug: %v", text, err, collectObjectTypeQuickstartDebug(page))
	}
}

// Reload the page and wait for the app and route to be ready again.
func reloadObjectTypeRoute(t testing.TB, page playwright.Page, assertReady func()) {
	t.Helper()

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload object route: %v", err)
	}
	WaitForApp(t, page)
	assertReady()
}

// Run the quickstart helper script and type-check its result.
func runObjectTypeQuickstartScript(t testing.TB, h *Harness, page playwright.Page, args map[string]any) map[string]any {
	// Mark the function as a test helper.
	t.Helper()

	// Evaluate the helper script in the page.
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
	// Read the object seqno through the quickstart helper script.
	t.Helper()

	// Run the seqno helper action and validate the result.
	result := runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "seqno",
	})
	seqno := stringField(result, "seqno")
	if seqno == "" {
		t.Fatalf("helper returned empty seqno: %#v", result)
	}
	return seqno
}

// Read a KV value through the quickstart helper script.
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

// Assert the KV helper found the key with the expected value.
func assertKvValue(t testing.TB, result map[string]any, key, want string) {
	t.Helper()

	if !boolField(result, "found") {
		t.Fatalf("KV key %q was not found: %#v", key, result)
	}
	if got := stringField(result, "value"); got != want {
		t.Fatalf("KV key %q value = %q, want %q: %#v", key, got, want, result)
	}
}

// Read the SQL query-result linkage through the helper script.
func readSqlQueryResultLinkage(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	return runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "sql-linkage",
	})
}

// Prepare SQL workbench pins through the helper script.
func prepareSqlWorkbenchPins(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	return runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "prepare-workbench-pins",
	})
}

// Read SQL workbench pins through the helper script.
func readSqlWorkbenchPins(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	return runObjectTypeQuickstartScript(t, h, page, map[string]any{
		"action": "workbench-pins",
	})
}

// Assert the workbench target database and pinned queries.
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

// Read a string slice field from the helper result map.
func stringSliceField(t testing.TB, m map[string]any, key string) []string {
	// Mark the function as a test helper.
	t.Helper()

	// Extract the raw slice and validate each item is a string.
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

// Assert two string slices contain the same set of values.
func assertStringSet(t testing.TB, got, want []string) {
	// Mark the function as a test helper.
	t.Helper()

	// Compare lengths, then check every wanted value is present.
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

// Drain the console and fail if a browser crash was reported.
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

// Collect object type quickstart debug state from the page.
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
